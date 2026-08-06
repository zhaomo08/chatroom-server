package message

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"chatroom-server/internal/auth"
	"chatroom-server/internal/room"
)

// Broadcaster pushes an already-serialized message payload to a set of uids
// (or to everyone, for hot rooms). Implemented by ws.Hub.
type Broadcaster interface {
	SendToUsers(uids []int64, payload []byte)
	BroadcastAll(payload []byte)
}

type Handler struct {
	messages Store
	rooms    room.Store
	hub      Broadcaster
}

func NewHandler(messages Store, rooms room.Store, hub Broadcaster) *Handler {
	return &Handler{messages: messages, rooms: rooms, hub: hub}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/messages", h.send)
	mux.HandleFunc("PUT /api/messages/{id}/recall", h.recall)
	mux.HandleFunc("PUT /api/messages/{id}/mark", h.mark)
	mux.HandleFunc("GET /api/rooms/{roomID}/messages", h.page)
}

type sendRequest struct {
	RoomID     int64           `json:"room_id"`
	Content    string          `json:"content"`
	Type       Type            `json:"type"`
	ReplyMsgID int64           `json:"reply_msg_id"`
	Extra      json.RawMessage `json:"extra"`
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())

	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RoomID == 0 {
		writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}
	if req.Type == 0 {
		req.Type = TypeText
	}
	switch req.Type {
	case TypeText, TypeEmoji:
		if req.Content == "" {
			writeError(w, http.StatusBadRequest, "content is required")
			return
		}
	case TypeImage, TypeVideo:
		if len(req.Extra) == 0 {
			writeError(w, http.StatusBadRequest, "extra with file_id is required")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "unsupported message type")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rm, err := h.rooms.GetRoom(ctx, req.RoomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}
	if !h.canPost(ctx, *rm, uid) {
		writeError(w, http.StatusForbidden, "not a participant of this room")
		return
	}

	msg := &Message{RoomID: req.RoomID, FromUID: uid, Content: req.Content, Type: req.Type, ReplyMsgID: req.ReplyMsgID}
	if len(req.Extra) > 0 {
		extra := string(req.Extra)
		msg.Extra = &extra
	}
	id, err := h.messages.Insert(ctx, msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}
	msg.ID = id

	h.broadcast(ctx, *rm, msg)

	writeJSON(w, http.StatusOK, msg)
}

// canPost reports whether uid may send a message into rm. Hot rooms are
// public discussion rooms by design, so any authenticated user may post;
// ordinary groups require membership and 1:1 rooms require being one of the
// two participants.
func (h *Handler) canPost(ctx context.Context, rm room.Room, uid int64) bool {
	if rm.IsHot() {
		return true
	}
	if rm.IsGroup() {
		_, err := h.rooms.GetMember(ctx, rm.ID, uid)
		return err == nil
	}
	friend, err := h.rooms.GetFriendByRoomID(ctx, rm.ID)
	if err != nil {
		return false
	}
	return friend.UID1 == uid || friend.UID2 == uid
}

func (h *Handler) broadcast(ctx context.Context, rm room.Room, msg *Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if rm.IsHot() {
		h.hub.BroadcastAll(payload)
		return
	}
	var recipients []int64
	if rm.IsGroup() {
		members, err := h.rooms.ListMembers(ctx, rm.ID)
		if err != nil {
			return
		}
		uids := make([]int64, len(members))
		for i, m := range members {
			uids[i] = m.UID
		}
		recipients = room.Recipients(rm, uids, nil)
	} else {
		friend, err := h.rooms.GetFriendByRoomID(ctx, rm.ID)
		if err != nil {
			return
		}
		recipients = room.Recipients(rm, nil, friend)
	}
	h.hub.SendToUsers(recipients, payload)
}

func (h *Handler) recall(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())
	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	msg, err := h.messages.GetByID(ctx, msgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}

	actorRole := room.RoleMember
	if member, err := h.rooms.GetMember(ctx, msg.RoomID, uid); err == nil {
		actorRole = member.Role
	}

	if !CanRecall(msg.FromUID, uid, actorRole, msg.CreateTime, time.Now()) {
		writeError(w, http.StatusForbidden, "not allowed to recall this message")
		return
	}

	if err := h.messages.SetStatus(ctx, msgID, StatusDeleted); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to recall message")
		return
	}

	msg.Status = StatusDeleted
	if rm, err := h.rooms.GetRoom(ctx, msg.RoomID); err == nil {
		h.broadcast(ctx, *rm, msg)
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type markRequest struct {
	Type int `json:"type"`
}

func (h *Handler) mark(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())
	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var req markRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.messages.AddMark(ctx, msgID, uid, req.Type); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark message")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	roomID, err := strconv.ParseInt(r.PathValue("roomID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid roomID")
		return
	}
	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before_id"), 10, 64)
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	msgs, err := h.messages.ListByRoomCursor(ctx, roomID, beforeID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status, "msg": msg})
}
