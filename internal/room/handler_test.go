package room

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"chatroom-server/internal/auth"
)

type fakeStore struct {
	rooms   map[int64]*Room
	groups  map[int64]*Group
	friends map[int64]*Friend
	members map[int64]map[int64]Member // groupID -> uid -> Member
	nextID  int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		rooms:   map[int64]*Room{},
		groups:  map[int64]*Group{},
		friends: map[int64]*Friend{},
		members: map[int64]map[int64]Member{},
	}
}

func (f *fakeStore) CreateGroupRoom(ctx context.Context, ownerUID int64, name, avatar string) (int64, error) {
	f.nextID++
	roomID := f.nextID
	f.rooms[roomID] = &Room{ID: roomID, Type: TypeGroup}
	f.groups[roomID] = &Group{ID: roomID, RoomID: roomID, Name: name, Avatar: avatar}
	f.members[roomID] = map[int64]Member{ownerUID: {GroupID: roomID, UID: ownerUID, Role: RoleOwner}}
	return roomID, nil
}

func (f *fakeStore) AddMember(ctx context.Context, groupID, uid int64, role Role) error {
	f.members[groupID][uid] = Member{GroupID: groupID, UID: uid, Role: role}
	return nil
}

func (f *fakeStore) RemoveMember(ctx context.Context, groupID, uid int64) error {
	delete(f.members[groupID], uid)
	return nil
}

func (f *fakeStore) SetRole(ctx context.Context, groupID, uid int64, role Role) error {
	m := f.members[groupID][uid]
	m.Role = role
	f.members[groupID][uid] = m
	return nil
}

func (f *fakeStore) ListMembers(ctx context.Context, groupID int64) ([]Member, error) {
	var out []Member
	for _, m := range f.members[groupID] {
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeStore) GetMember(ctx context.Context, groupID, uid int64) (*Member, error) {
	m, ok := f.members[groupID][uid]
	if !ok {
		return nil, ErrNotFound
	}
	return &m, nil
}

func (f *fakeStore) GetRoom(ctx context.Context, roomID int64) (*Room, error) {
	r, ok := f.rooms[roomID]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (f *fakeStore) GetGroupByRoomID(ctx context.Context, roomID int64) (*Group, error) {
	g, ok := f.groups[roomID]
	if !ok {
		return nil, ErrNotFound
	}
	return g, nil
}

func (f *fakeStore) GetFriendByRoomID(ctx context.Context, roomID int64) (*Friend, error) {
	fr, ok := f.friends[roomID]
	if !ok {
		return nil, ErrNotFound
	}
	return fr, nil
}

func TestCreateGroupAndAddMember(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	h := NewHandler(store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	validToken, _ := auth.GenerateToken(1, secret, 3600_000_000_000)
	body, _ := json.Marshal(map[string]string{"name": "My Group"})
	req := httptest.NewRequest(http.MethodPost, "/api/rooms/groups", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("createGroup status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created map[string]int64
	json.NewDecoder(rec.Body).Decode(&created)
	roomID := created["room_id"]

	body, _ = json.Marshal(map[string]int64{"uid": 2})
	req = httptest.NewRequest(http.MethodPost, "/api/rooms/groups/"+strconv.FormatInt(roomID, 10)+"/members", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("addMember status = %d, body = %s", rec.Code, rec.Body.String())
	}

	members, _ := store.ListMembers(context.Background(), roomID)
	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(members))
	}
}

func TestRemoveMemberForbiddenForRegularMember(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	roomID, _ := store.CreateGroupRoom(context.Background(), 1, "g", "")
	store.AddMember(context.Background(), roomID, 2, RoleMember)
	store.AddMember(context.Background(), roomID, 3, RoleMember)

	h := NewHandler(store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	// uid 2 (a regular member) tries to remove uid 3 (also a regular member) - must be forbidden.
	actorToken, _ := auth.GenerateToken(2, secret, 3600_000_000_000)
	req := httptest.NewRequest(http.MethodDelete, "/api/rooms/groups/"+strconv.FormatInt(roomID, 10)+"/members/3", nil)
	req.Header.Set("Authorization", "Bearer "+actorToken)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestOwnerCanRemoveAdmin(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	roomID, _ := store.CreateGroupRoom(context.Background(), 1, "g", "")
	store.AddMember(context.Background(), roomID, 2, RoleAdmin)

	h := NewHandler(store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	ownerToken, _ := auth.GenerateToken(1, secret, 3600_000_000_000)
	req := httptest.NewRequest(http.MethodDelete, "/api/rooms/groups/"+strconv.FormatInt(roomID, 10)+"/members/2", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSetAdminForbiddenForNonOwner(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	roomID, _ := store.CreateGroupRoom(context.Background(), 1, "g", "")
	store.AddMember(context.Background(), roomID, 2, RoleMember)
	store.AddMember(context.Background(), roomID, 3, RoleMember)

	h := NewHandler(store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	memberToken, _ := auth.GenerateToken(2, secret, 3600_000_000_000)
	body, _ := json.Marshal(map[string]bool{"is_admin": true})
	req := httptest.NewRequest(http.MethodPut, "/api/rooms/groups/"+strconv.FormatInt(roomID, 10)+"/admins/3", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+memberToken)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
