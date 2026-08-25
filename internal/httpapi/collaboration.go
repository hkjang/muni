package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Binary frames carry a one byte channel prefix.
const (
	channelUpdate    = 0
	channelAwareness = 1
	channelSnapshot  = 2
)

var collabUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 << 10,
	WriteBufferSize: 32 << 10,
	CheckOrigin: func(r *http.Request) bool {
		return sameOrigin(r)
	},
}

func (s *Server) collaboration(w http.ResponseWriter, r *http.Request) {
	documentID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	role, err := s.documentRole(r.Context(), p.User, documentID, false)
	if err != nil {
		writeError(w, http.StatusForbidden, "DOCUMENT_PERMISSION_DENIED", "문서 공동편집에 참여할 권한이 없습니다.")
		return
	}
	var generation int
	var workflowStatus string
	if err := s.db.QueryRow(r.Context(), `SELECT crdt_generation,workflow_status FROM documents WHERE id=$1`, documentID).Scan(&generation, &workflowStatus); err != nil {
		writeError(w, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "문서를 찾을 수 없습니다.")
		return
	}
	state, err := s.loadCollabState(r.Context(), documentID, generation)
	if err != nil {
		writeError(w, 500, "COLLAB_LOAD_FAILED", "공동편집 상태를 불러오지 못했습니다.")
		return
	}
	updates := make([]string, 0, len(state.updates))
	for _, update := range state.updates {
		updates = append(updates, base64.StdEncoding.EncodeToString(update))
	}
	snapshot := ""
	if len(state.snapshot) > 0 {
		snapshot = base64.StdEncoding.EncodeToString(state.snapshot)
	}

	conn, err := collabUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	leave := s.hub.Join(documentID, conn)
	defer leave()
	writeAllowed := roleRank[role] >= roleRank["EDITOR"] && workflowStatus != "PENDING"
	// Only a client that may write is asked to compact: the snapshot it sends
	// back replaces the stored history.
	compact := writeAllowed && shouldCompact(len(state.updates), state.bytes)
	// An empty shared state has to be filled from the stored document, and only
	// one client may do it or the content is inserted twice.
	seed := writeAllowed && snapshot == "" && len(updates) == 0 &&
		s.hub.ClaimSeed(documentID, generation, conn)
	initial, _ := json.Marshal(map[string]any{
		"type": "sync", "generation": generation,
		"snapshot": snapshot, "updates": updates,
		"baseSeq": state.maxSeq, "compact": compact, "seed": seed,
		"permission": role, "writeAllowed": writeAllowed,
		"user": map[string]any{"id": p.User.ID, "displayName": p.User.DisplayName},
	})
	if err := s.hub.Send(documentID, conn, websocket.TextMessage, initial); err != nil {
		return
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if s.hub.Send(documentID, conn, websocket.PingMessage, nil) != nil {
					return
				}
			}
		}
	}()

	// A compaction snapshot is a whole document state, so it needs more room
	// than a single incremental update; per-channel limits are enforced below.
	conn.SetReadLimit(maxSnapshotBytes + 8)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		if messageType == websocket.BinaryMessage {
			if len(payload) < 2 {
				continue
			}
			channel := payload[0]
			body := payload[1:]
			switch channel {
			case channelUpdate:
				if !writeAllowed || len(body) > maxUpdateBytes {
					continue
				}
				if _, err := s.db.Exec(r.Context(), `INSERT INTO collab_updates(document_id,generation,author_id,update_data) VALUES($1,$2,$3,$4)`, documentID, generation, p.User.ID, body); err != nil {
					s.logger.Warn("collaboration update was not persisted", "document_id", documentID, "error", err)
					continue
				}
				// The state is no longer empty, so nobody needs to seed it.
				s.hub.ReleaseSeed(documentID)
				s.hub.Broadcast(documentID, conn, websocket.BinaryMessage, payload)
			case channelAwareness:
				s.hub.Broadcast(documentID, conn, websocket.BinaryMessage, payload)
			case channelSnapshot:
				// The client merged everything the server sent into one state.
				// It is not a delta, so the other clients already have it and
				// it is never broadcast.
				if !compact || !writeAllowed || len(body) == 0 || len(body) > maxSnapshotBytes {
					continue
				}
				if err := s.storeCollabSnapshot(r.Context(), documentID, generation, state.maxSeq, p.User.ID, body); err != nil {
					s.logger.Warn("collaboration snapshot was not stored", "document_id", documentID, "error", err)
					continue
				}
				s.logger.Info("collaboration history compacted", "document_id", documentID, "generation", generation, "base_seq", state.maxSeq, "updates", len(state.updates), "bytes", len(body))
				compact = false
			}
		} else if messageType == websocket.TextMessage {
			// Text presence/status messages are ephemeral and never persisted.
			if len(payload) <= 16<<10 {
				s.hub.Broadcast(documentID, conn, websocket.TextMessage, payload)
			}
		}
	}
}
