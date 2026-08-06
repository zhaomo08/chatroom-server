package call

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatroom-server/internal/auth"
	"chatroom-server/internal/room"
)

type fakeRoomStore struct {
	rooms   map[int64]*room.Room
	members map[int64][]room.Member
	friends map[int64]*room.Friend
}

func (f *fakeRoomStore) CreateGroupRoom(ctx context.Context, ownerUID int64, name, avatar string) (int64, error) {
	return 0, nil
}
func (f *fakeRoomStore) AddMember(ctx context.Context, groupID, uid int64, role room.Role) error {
	return nil
}
func (f *fakeRoomStore) RemoveMember(ctx context.Context, groupID, uid int64) error { return nil }
func (f *fakeRoomStore) SetRole(ctx context.Context, groupID, uid int64, role room.Role) error {
	return nil
}
func (f *fakeRoomStore) ListMembers(ctx context.Context, groupID int64) ([]room.Member, error) {
	return f.members[groupID], nil
}
func (f *fakeRoomStore) GetMember(ctx context.Context, groupID, uid int64) (*room.Member, error) {
	for _, m := range f.members[groupID] {
		if m.UID == uid {
			return &m, nil
		}
	}
	return nil, room.ErrNotFound
}
func (f *fakeRoomStore) GetRoom(ctx context.Context, roomID int64) (*room.Room, error) {
	r, ok := f.rooms[roomID]
	if !ok {
		return nil, room.ErrNotFound
	}
	return r, nil
}
func (f *fakeRoomStore) GetGroupByRoomID(ctx context.Context, roomID int64) (*room.Group, error) {
	return nil, room.ErrNotFound
}
func (f *fakeRoomStore) GetFriendByRoomID(ctx context.Context, roomID int64) (*room.Friend, error) {
	fr, ok := f.friends[roomID]
	if !ok {
		return nil, room.ErrNotFound
	}
	return fr, nil
}
func (f *fakeRoomStore) ListRoomsForUser(ctx context.Context, uid int64) ([]room.RoomSummary, error) {
	return nil, nil
}
func (f *fakeRoomStore) GetOrCreateFriendRoom(ctx context.Context, uid1, uid2 int64) (int64, error) {
	return 0, nil
}

type fakeHub struct {
	sentTo map[int64][][]byte
}

func newFakeHub() *fakeHub { return &fakeHub{sentTo: map[int64][][]byte{}} }

func (h *fakeHub) SendToUsers(uids []int64, payload []byte) {
	for _, uid := range uids {
		h.sentTo[uid] = append(h.sentTo[uid], payload)
	}
}

func TestTokenIssuedForParticipant(t *testing.T) {
	secret := []byte("test-secret")
	store := &fakeRoomStore{
		rooms:   map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {{GroupID: 10, UID: 1, Role: room.RoleOwner}}},
	}
	h := NewHandler(store, newFakeHub(), "devkey", "devsecret", "ws://localhost:7880")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	body, _ := json.Marshal(map[string]int64{"room_id": 10})
	req := httptest.NewRequest(http.MethodPost, "/api/calls/token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp tokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected a non-empty token")
	}
	if resp.URL != "ws://localhost:7880" {
		t.Errorf("URL = %q, want ws://localhost:7880", resp.URL)
	}
}

func TestTokenForbiddenForNonParticipant(t *testing.T) {
	secret := []byte("test-secret")
	store := &fakeRoomStore{
		rooms:   map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {{GroupID: 10, UID: 1, Role: room.RoleOwner}}},
	}
	h := NewHandler(store, newFakeHub(), "devkey", "devsecret", "ws://localhost:7880")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	// uid 2 is not a member of room 10.
	token, _ := auth.GenerateToken(2, secret, time.Hour)
	body, _ := json.Marshal(map[string]int64{"room_id": 10})
	req := httptest.NewRequest(http.MethodPost, "/api/calls/token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

func TestInviteBroadcastsToOtherGroupMembers(t *testing.T) {
	secret := []byte("test-secret")
	store := &fakeRoomStore{
		rooms: map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {
			{GroupID: 10, UID: 1, Role: room.RoleOwner},
			{GroupID: 10, UID: 2, Role: room.RoleMember},
			{GroupID: 10, UID: 3, Role: room.RoleMember},
		}},
	}
	hub := newFakeHub()
	h := NewHandler(store, hub, "devkey", "devsecret", "ws://localhost:7880")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	body, _ := json.Marshal(map[string]any{"room_id": 10, "mode": "video"})
	req := httptest.NewRequest(http.MethodPost, "/api/calls/invite", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(hub.sentTo[2]) != 1 || len(hub.sentTo[3]) != 1 {
		t.Fatalf("expected uid 2 and 3 (not the caller) to receive the invite, got sentTo = %+v", hub.sentTo)
	}
	if len(hub.sentTo[1]) != 0 {
		t.Error("caller should not receive their own invite")
	}

	var envelope struct {
		Kind    string       `json:"kind"`
		Payload Notification `json:"payload"`
	}
	json.Unmarshal(hub.sentTo[2][0], &envelope)
	if envelope.Kind != "call_invite" {
		t.Errorf("Kind = %q, want call_invite", envelope.Kind)
	}
	if envelope.Payload.FromUID != 1 || envelope.Payload.Mode != "video" || envelope.Payload.RoomID != 10 {
		t.Errorf("Notification = %+v, unexpected", envelope.Payload)
	}
}

func TestInviteForbiddenForNonParticipant(t *testing.T) {
	secret := []byte("test-secret")
	store := &fakeRoomStore{
		rooms:   map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {{GroupID: 10, UID: 1, Role: room.RoleOwner}}},
	}
	h := NewHandler(store, newFakeHub(), "devkey", "devsecret", "ws://localhost:7880")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(99, secret, time.Hour)
	body, _ := json.Marshal(map[string]any{"room_id": 10, "mode": "audio"})
	req := httptest.NewRequest(http.MethodPost, "/api/calls/invite", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
