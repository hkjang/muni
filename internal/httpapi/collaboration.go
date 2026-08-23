package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
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
	rows, err := s.db.Query(r.Context(), `SELECT update_data FROM collab_updates WHERE document_id=$1 AND generation=$2 ORDER BY seq`, documentID, generation)
	if err != nil {
		writeError(w, 500, "COLLAB_LOAD_FAILED", "공동편집 상태를 불러오지 못했습니다.")
		return
	}
	updates := make([]string, 0)
	for rows.Next() {
		var update []byte
		if rows.Scan(&update) == nil {
			updates = append(updates, base64.StdEncoding.EncodeToString(update))
		}
	}
	rows.Close()

	conn, err := collabUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	leave := s.hub.Join(documentID, conn)
	defer leave()
	writeAllowed := roleRank[role] >= roleRank["EDITOR"] && workflowStatus != "PENDING"
	initial, _ := json.Marshal(map[string]any{"type": "sync", "generation": generation, "updates": updates, "permission": role, "writeAllowed": writeAllowed, "user": map[string]any{"id": p.User.ID, "displayName": p.User.DisplayName}})
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

	conn.SetReadLimit((1 << 20) + 1)
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
			if channel == 0 {
				if !writeAllowed {
					continue
				}
				if len(payload)-1 > 1<<20 {
					continue
				}
				if _, err := s.db.Exec(r.Context(), `INSERT INTO collab_updates(document_id,generation,author_id,update_data) VALUES($1,$2,$3,$4)`, documentID, generation, p.User.ID, payload[1:]); err != nil {
					s.logger.Warn("collaboration update was not persisted", "document_id", documentID, "error", err)
					continue
				}
			}
			if channel == 0 || channel == 1 {
				s.hub.Broadcast(documentID, conn, websocket.BinaryMessage, payload)
			}
		} else if messageType == websocket.TextMessage {
			// Text presence/status messages are ephemeral and never persisted.
			if len(payload) <= 16<<10 {
				s.hub.Broadcast(documentID, conn, websocket.TextMessage, payload)
			}
		}
	}
}
