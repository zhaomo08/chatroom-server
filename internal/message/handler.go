package message

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"time"

	"chatroom-server/internal/auth"
	"chatroom-server/internal/room"
	"chatroom-server/internal/ws"
)

// Broadcaster pushes an already-serialized message payload to a set of uids
// (or to everyone, for hot rooms). Implemented by ws.Hub.
type Broadcaster interface {
	SendToUsers(uids []int64, payload []byte)
	BroadcastAll(payload []byte)
}

// GroupMemberCache is a read-through cache for group membership, keyed by
// group id. Implemented by internal/cache.MemberCache. Pass nil to always
// hit the database (used by tests).
type GroupMemberCache interface {
	Get(ctx context.Context, groupID int64) ([]int64, bool, error)
	Set(ctx context.Context, groupID int64, uids []int64, ttl time.Duration) error
}

const groupMemberCacheTTL = 5 * time.Minute

type Handler struct {
	messages Store
	rooms    room.Store
	hub      Broadcaster
	cache    GroupMemberCache
}

func NewHandler(messages Store, rooms room.Store, hub Broadcaster, cache GroupMemberCache) *Handler {
	return &Handler{messages: messages, rooms: rooms, hub: hub, cache: cache}
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
	// Insert only returns the new id; the DB fills create_time via its own
	// DEFAULT CURRENT_TIMESTAMP, which this in-memory struct never sees.
	// Without this, the HTTP response and WS broadcast for a just-sent
	// message would carry the Go zero time, breaking any client-side
	// timestamp/date-grouping display until the room is reloaded from GET.
	msg.CreateTime = time.Now()

	h.broadcast(ctx, *rm, msg)
	h.trackActivity(ctx, *rm, uid, msg.ID)

	writeJSON(w, http.StatusOK, msg)
}

// canPost intentionally doesn't call room.IsParticipant: that helper
// re-queries group_member on every call, and canPost + broadcast both need
// the member list for the same request. groupMemberUIDs below fetches it
// once (cached) and both call sites reuse it.
func (h *Handler) canPost(ctx context.Context, rm room.Room, uid int64) bool {
	if rm.IsHot() {
		return true
	}
	if rm.IsGroup() {
		uids, err := h.groupMemberUIDs(ctx, rm.ID)
		if err != nil {
			return false
		}
		return slices.Contains(uids, uid)
	}
	friend, err := h.rooms.GetFriendByRoomID(ctx, rm.ID)
	if err != nil {
		return false
	}
	return friend.UID1 == uid || friend.UID2 == uid
}

// groupMemberUIDs returns every member uid of groupID, going through h.cache
// first when configured. A single message send calls this from both
// canPost and broadcast; the second call is a cache hit even within the
// same request, so a group send does at most one group_member query
// instead of two — and repeat sends in the same group do zero until the
// cache entry expires or membership changes (see room.Handler's
// invalidateCache).
func (h *Handler) groupMemberUIDs(ctx context.Context, groupID int64) ([]int64, error) {
	if h.cache != nil {
		if uids, ok, err := h.cache.Get(ctx, groupID); err == nil && ok {
			return uids, nil
		}
	}
	members, err := h.rooms.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	uids := make([]int64, len(members))
	for i, m := range members {
		uids[i] = m.UID
	}
	if h.cache != nil {
		_ = h.cache.Set(ctx, groupID, uids, groupMemberCacheTTL)
	}
	return uids, nil
}

func (h *Handler) broadcast(ctx context.Context, rm room.Room, msg *Message) {
	payload := ws.Marshal("chat_message", msg)
	if payload == nil {
		return
	}
	if rm.IsHot() {
		h.hub.BroadcastAll(payload)
		return
	}
	recipients, err := h.resolveRecipients(ctx, rm)
	if err != nil {
		return
	}
	h.hub.SendToUsers(recipients, payload)
}

// resolveRecipients is shared by broadcast (who gets the WS push) and
// trackActivity (whose unread count goes up) so a send resolves the
// room's member/friend list once — the second call is a cache hit for
// groups (see groupMemberUIDs).
func (h *Handler) resolveRecipients(ctx context.Context, rm room.Room) ([]int64, error) {
	if rm.IsGroup() {
		uids, err := h.groupMemberUIDs(ctx, rm.ID)
		if err != nil {
			return nil, err
		}
		return room.Recipients(rm, uids, nil), nil
	}
	friend, err := h.rooms.GetFriendByRoomID(ctx, rm.ID)
	if err != nil {
		return nil, err
	}
	return room.Recipients(rm, nil, friend), nil
}

// trackActivity bumps the room's recency/last-message pointer and every
// recipient's unread count for a genuinely new message. Deliberately not
// called from recall(): recalling an existing message shouldn't bump the
// room to the top of the list or create a fresh unread notification for
// something the recipient may have already read.
func (h *Handler) trackActivity(ctx context.Context, rm room.Room, senderUID, msgID int64) {
	_ = h.rooms.TouchRoom(ctx, rm.ID, msgID)
	if rm.IsHot() {
		return // hot rooms aren't part of anyone's personal room list/unread inbox
	}
	recipients, err := h.resolveRecipients(ctx, rm)
	if err != nil {
		return
	}
	others := make([]int64, 0, len(recipients))
	for _, uid := range recipients {
		if uid != senderUID {
			others = append(others, uid)
		}
	}
	_ = h.rooms.BumpUnread(ctx, rm.ID, others)
	_ = h.rooms.ResetUnread(ctx, senderUID, rm.ID)
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
