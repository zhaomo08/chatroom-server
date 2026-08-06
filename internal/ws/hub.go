package ws

import "sync"

// Conn is the minimal surface Hub needs from a connection; *websocket.Conn
// satisfies it. Kept as an interface so hub_test.go can use a fake.
type Conn interface {
	WriteMessage(messageType int, data []byte) error
}

type Hub struct {
	mu    sync.RWMutex
	conns map[int64]map[Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{conns: map[int64]map[Conn]struct{}{}}
}

func (h *Hub) Register(uid int64, c Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[uid] == nil {
		h.conns[uid] = map[Conn]struct{}{}
	}
	h.conns[uid][c] = struct{}{}
}

func (h *Hub) Unregister(uid int64, c Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[uid], c)
	if len(h.conns[uid]) == 0 {
		delete(h.conns, uid)
	}
}

func (h *Hub) SendToUsers(uids []int64, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, uid := range uids {
		for c := range h.conns[uid] {
			c.WriteMessage(1, payload) // 1 = websocket.TextMessage
		}
	}
}

func (h *Hub) BroadcastAll(payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conns := range h.conns {
		for c := range conns {
			c.WriteMessage(1, payload)
		}
	}
}
