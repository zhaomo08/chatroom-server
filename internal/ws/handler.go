package ws

import (
	"net/http"

	"github.com/gorilla/websocket"

	"chatroom-server/internal/auth"
)

type Handler struct {
	hub      *Hub
	secret   []byte
	upgrader websocket.Upgrader
}

func NewHandler(hub *Hub, secret []byte) *Handler {
	return &Handler{hub: hub, secret: secret, upgrader: websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws", h.serveWS)
}

func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	uid, err := auth.ParseToken(r.URL.Query().Get("token"), h.secret)
	if err != nil {
		http.Error(w, `{"code":401,"msg":"invalid token"}`, http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.hub.Register(uid, conn)
	defer h.hub.Unregister(uid, conn)

	// Read (and discard) until the client disconnects, so the server notices
	// closed connections and cleans up the Hub entry. This connection is
	// receive-only from the client's perspective; sending happens elsewhere
	// via Hub.SendToUsers/BroadcastAll.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
