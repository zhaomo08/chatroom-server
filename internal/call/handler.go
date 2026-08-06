package call

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"chatroom-server/internal/auth"
	"chatroom-server/internal/room"
	"chatroom-server/internal/ws"
)

// Broadcaster pushes an already-serialized payload to a set of uids.
// Implemented by ws.Hub.
type Broadcaster interface {
	SendToUsers(uids []int64, payload []byte)
}

type Handler struct {
	rooms     room.Store
	hub       Broadcaster
	apiKey    string
	apiSecret string
	publicURL string
}

func NewHandler(rooms room.Store, hub Broadcaster, apiKey, apiSecret, publicURL string) *Handler {
	return &Handler{rooms: rooms, hub: hub, apiKey: apiKey, apiSecret: apiSecret, publicURL: publicURL}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/calls/token", h.token)
	mux.HandleFunc("POST /api/calls/invite", h.invite)
}

type tokenRequest struct {
	RoomID int64 `json:"room_id"`
}

type tokenResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// roomName maps our internal room id to a LiveKit room name.
func roomName(roomID int64) string {
	return "room-" + strconv.FormatInt(roomID, 10)
}

func (h *Handler) token(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())

	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RoomID == 0 {
		writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rm, err := h.rooms.GetRoom(ctx, req.RoomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}
	if !room.IsParticipant(ctx, h.rooms, *rm, uid) {
		writeError(w, http.StatusForbidden, "not a participant of this room")
		return
	}

	tok, err := mintToken(h.apiKey, h.apiSecret, strconv.FormatInt(uid, 10), roomName(req.RoomID), time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mint token")
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{Token: tok, URL: h.publicURL})
}

type inviteRequest struct {
	RoomID int64  `json:"room_id"`
	Mode   string `json:"mode"` // "audio" | "video"
}

// Notification is pushed over the chat WebSocket (kind "call_invite") to
// every other participant of the room when someone starts a call.
type Notification struct {
	RoomID  int64  `json:"room_id"`
	FromUID int64  `json:"from_uid"`
	Mode    string `json:"mode"`
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())

	var req inviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RoomID == 0 {
		writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}
	if req.Mode != "audio" && req.Mode != "video" {
		req.Mode = "audio"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rm, err := h.rooms.GetRoom(ctx, req.RoomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}
	if !room.IsParticipant(ctx, h.rooms, *rm, uid) {
		writeError(w, http.StatusForbidden, "not a participant of this room")
		return
	}

	recipients := h.otherParticipants(ctx, *rm, uid)
	payload := ws.Marshal("call_invite", Notification{RoomID: req.RoomID, FromUID: uid, Mode: req.Mode})
	h.hub.SendToUsers(recipients, payload)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) otherParticipants(ctx context.Context, rm room.Room, uid int64) []int64 {
	if rm.IsGroup() {
		members, err := h.rooms.ListMembers(ctx, rm.ID)
		if err != nil {
			return nil
		}
		var out []int64
		for _, m := range members {
			if m.UID != uid {
				out = append(out, m.UID)
			}
		}
		return out
	}
	friend, err := h.rooms.GetFriendByRoomID(ctx, rm.ID)
	if err != nil {
		return nil
	}
	peer := friend.UID1
	if peer == uid {
		peer = friend.UID2
	}
	return []int64{peer}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status, "msg": msg})
}
