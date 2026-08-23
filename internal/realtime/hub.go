package realtime

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Hub struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]map[*websocket.Conn]*sync.Mutex
}

// CloseDocument forces connected clients to re-authorize after a workflow or
// permission transition. It also prevents an already-open editor socket from
// continuing to write while a document is frozen for approval.
func (h *Hub) CloseDocument(documentID uuid.UUID) {
	h.mu.Lock()
	connections := h.rooms[documentID]
	delete(h.rooms, documentID)
	h.mu.Unlock()
	for conn, mu := range connections {
		mu.Lock()
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "document state changed"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		mu.Unlock()
	}
}

func NewHub() *Hub { return &Hub{rooms: make(map[uuid.UUID]map[*websocket.Conn]*sync.Mutex)} }

func (h *Hub) Join(documentID uuid.UUID, conn *websocket.Conn) func() {
	h.mu.Lock()
	if h.rooms[documentID] == nil {
		h.rooms[documentID] = make(map[*websocket.Conn]*sync.Mutex)
	}
	h.rooms[documentID][conn] = &sync.Mutex{}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.rooms[documentID], conn)
		if len(h.rooms[documentID]) == 0 {
			delete(h.rooms, documentID)
		}
		h.mu.Unlock()
		_ = conn.Close()
	}
}

func (h *Hub) Broadcast(documentID uuid.UUID, sender *websocket.Conn, messageType int, payload []byte) {
	h.mu.RLock()
	type target struct {
		conn *websocket.Conn
		mu   *sync.Mutex
	}
	connections := make([]target, 0, len(h.rooms[documentID]))
	for conn, mu := range h.rooms[documentID] {
		if conn != sender {
			connections = append(connections, target{conn: conn, mu: mu})
		}
	}
	h.mu.RUnlock()
	for _, item := range connections {
		item.mu.Lock()
		_ = item.conn.WriteMessage(messageType, payload)
		item.mu.Unlock()
	}
}

func (h *Hub) Send(documentID uuid.UUID, conn *websocket.Conn, messageType int, payload []byte) error {
	h.mu.RLock()
	mu := h.rooms[documentID][conn]
	h.mu.RUnlock()
	if mu == nil {
		return websocket.ErrCloseSent
	}
	mu.Lock()
	defer mu.Unlock()
	return conn.WriteMessage(messageType, payload)
}
