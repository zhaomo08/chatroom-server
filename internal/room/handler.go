package room

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"chatroom-server/internal/auth"
)

// MemberCache is the write side of message.GroupMemberCache: room.Handler
// invalidates a group's cached member list whenever membership actually
// changes, so message.Handler's read-through cache doesn't serve a stale
// list to a newly-added or just-removed member. Implemented by
// internal/cache.MemberCache. Pass nil to disable (used by tests).
type MemberCache interface {
	Invalidate(ctx context.Context, groupID int64) error
}

type Handler struct {
	store Store
	cache MemberCache
}

func NewHandler(store Store, cache MemberCache) *Handler { return &Handler{store: store, cache: cache} }

func (h *Handler) invalidateCache(ctx context.Context, groupID int64) {
	if h.cache == nil {
		return
	}
	_ = h.cache.Invalidate(ctx, groupID)
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/rooms", h.listRooms)
	mux.HandleFunc("POST /api/rooms/friends", h.createFriendRoom)
	mux.HandleFunc("POST /api/rooms/groups", h.createGroup)
	mux.HandleFunc("PUT /api/rooms/{roomID}/read", h.markRead)
	mux.HandleFunc("POST /api/rooms/groups/{roomID}/members", h.addMember)
	mux.HandleFunc("DELETE /api/rooms/groups/{roomID}/members/{uid}", h.removeMember)
	mux.HandleFunc("DELETE /api/rooms/groups/{roomID}/members/me", h.exitGroup)
	mux.HandleFunc("PUT /api/rooms/groups/{roomID}/admins/{uid}", h.setAdmin)
	mux.HandleFunc("GET /api/rooms/groups/{roomID}/members", h.listMembers)
}

// markRead zeroes the caller's unread count for roomID (called when they
// open/view the room). No membership check: resetting an unread counter
// for a room you're not in is harmless housekeeping, not an access to
// anything private.
func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())
	roomID, err := strconv.ParseInt(r.PathValue("roomID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid roomID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.store.ResetUnread(ctx, uid, roomID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark room as read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rooms, err := h.store.ListRoomsForUser(ctx, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list rooms")
		return
	}
	writeJSON(w, http.StatusOK, rooms)
}

type createFriendRoomRequest struct {
	UID int64 `json:"uid"`
}

func (h *Handler) createFriendRoom(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())

	var req createFriendRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UID == 0 {
		writeError(w, http.StatusBadRequest, "uid is required")
		return
	}
	if req.UID == uid {
		writeError(w, http.StatusBadRequest, "cannot start a DM with yourself")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	roomID, err := h.store.GetOrCreateFriendRoom(ctx, uid, req.UID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start conversation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"room_id": roomID})
}

type createGroupRequest struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())

	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	roomID, err := h.store.CreateGroupRoom(ctx, uid, req.Name, req.Avatar)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"room_id": roomID})
}

type addMemberRequest struct {
	UID int64 `json:"uid"`
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := auth.UIDFromContext(r.Context())
	groupID, err := strconv.ParseInt(r.PathValue("roomID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid roomID")
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UID == 0 {
		writeError(w, http.StatusBadRequest, "uid is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := h.store.GetMember(ctx, groupID, actorUID); err != nil {
		writeError(w, http.StatusForbidden, "not a member of this group")
		return
	}

	if err := h.store.AddMember(ctx, groupID, req.UID, RoleMember); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	h.invalidateCache(ctx, groupID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := auth.UIDFromContext(r.Context())
	groupID, err := strconv.ParseInt(r.PathValue("roomID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid roomID")
		return
	}
	targetUID, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uid")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	actor, err := h.store.GetMember(ctx, groupID, actorUID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not a member of this group")
		return
	}
	target, err := h.store.GetMember(ctx, groupID, targetUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target is not a member")
		return
	}
	if !CanRemoveMember(actor.Role, target.Role) {
		writeError(w, http.StatusForbidden, "not allowed to remove this member")
		return
	}

	if err := h.store.RemoveMember(ctx, groupID, targetUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	h.invalidateCache(ctx, groupID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) exitGroup(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UIDFromContext(r.Context())
	groupID, err := strconv.ParseInt(r.PathValue("roomID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid roomID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	member, err := h.store.GetMember(ctx, groupID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, "not a member of this group")
		return
	}
	if member.Role == RoleOwner {
		writeError(w, http.StatusForbidden, "owner must transfer ownership or dismiss the group instead of exiting")
		return
	}

	if err := h.store.RemoveMember(ctx, groupID, uid); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to exit group")
		return
	}
	h.invalidateCache(ctx, groupID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type setAdminRequest struct {
	IsAdmin bool `json:"is_admin"`
}

func (h *Handler) setAdmin(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := auth.UIDFromContext(r.Context())
	groupID, err := strconv.ParseInt(r.PathValue("roomID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid roomID")
		return
	}
	targetUID, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uid")
		return
	}

	var req setAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	actor, err := h.store.GetMember(ctx, groupID, actorUID)
	if err != nil || !CanSetAdmin(actor.Role) {
		writeError(w, http.StatusForbidden, "only the owner can set admins")
		return
	}

	role := RoleMember
	if req.IsAdmin {
		role = RoleAdmin
	}
	if err := h.store.SetRole(ctx, groupID, targetUID, role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update role")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(r.PathValue("roomID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid roomID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	members, err := h.store.ListMembers(ctx, groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status, "msg": msg})
}
