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
	seeds map[uuid.UUID]seedClaim
}

// seedClaim records which connection was told to write the document's starting
// content into an empty shared state.
type seedClaim struct {
	generation int
	conn       *websocket.Conn
}

// CloseDocument forces connected clients to re-authorize after a workflow or
// permission transition. It also prevents an already-open editor socket from
// continuing to write while a document is frozen for approval.
func (h *Hub) CloseDocument(documentID uuid.UUID) {
	h.mu.Lock()
	connections := h.rooms[documentID]
	delete(h.rooms, documentID)
	delete(h.seeds, documentID)
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

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[uuid.UUID]map[*websocket.Conn]*sync.Mutex),
		seeds: make(map[uuid.UUID]seedClaim),
	}
}

// ClaimSeed picks the one client that fills an empty shared state.
//
// An empty state is what a brand new document and a just-restored one both look
// like, and the content has to be written by a browser because only a browser
// can encode a CRDT update. Two clients seeding the same empty document would
// each insert the whole text and the reader would see it twice, so exactly one
// is asked. The claim is released if that client leaves before it writes
// anything, otherwise a dropped connection would leave the document blank.
func (h *Hub) ClaimSeed(documentID uuid.UUID, generation int, conn *websocket.Conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	claim, exists := h.seeds[documentID]
	if exists && claim.generation == generation {
		if _, stillHere := h.rooms[documentID][claim.conn]; stillHere {
			return claim.conn == conn
		}
	}
	h.seeds[documentID] = seedClaim{generation: generation, conn: conn}
	return true
}

// ReleaseSeed forgets the claim once the state is no longer empty.
func (h *Hub) ReleaseSeed(documentID uuid.UUID) {
	h.mu.Lock()
	delete(h.seeds, documentID)
	h.mu.Unlock()
}

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

// Size reports how many documents are open and how many connections are on
// them, for the metrics an operator scrapes. It takes the read lock only, so
// scraping never delays an edit.
func (h *Hub) Size() (rooms int, connections int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, room := range h.rooms {
		rooms++
		connections += len(room)
	}
	return rooms, connections
}
