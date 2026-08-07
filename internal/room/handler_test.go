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
	unread  map[int64]map[int64]int    // roomID -> uid -> unread count
	nextID  int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		rooms:   map[int64]*Room{},
		groups:  map[int64]*Group{},
		friends: map[int64]*Friend{},
		members: map[int64]map[int64]Member{},
		unread:  map[int64]map[int64]int{},
	}
}

func (f *fakeStore) TouchRoom(ctx context.Context, roomID, msgID int64) error {
	if r, ok := f.rooms[roomID]; ok {
		r.LastMsgID = msgID
	}
	return nil
}

func (f *fakeStore) BumpUnread(ctx context.Context, roomID int64, recipientUIDs []int64) error {
	if f.unread[roomID] == nil {
		f.unread[roomID] = map[int64]int{}
	}
	for _, uid := range recipientUIDs {
		f.unread[roomID][uid]++
	}
	return nil
}

func (f *fakeStore) ResetUnread(ctx context.Context, uid, roomID int64) error {
	if f.unread[roomID] == nil {
		f.unread[roomID] = map[int64]int{}
	}
	f.unread[roomID][uid] = 0
	return nil
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

func (f *fakeStore) ListRoomsForUser(ctx context.Context, uid int64) ([]RoomSummary, error) {
	var out []RoomSummary
	for roomID, members := range f.members {
		if _, ok := members[uid]; ok {
			out = append(out, RoomSummary{RoomID: roomID, Type: TypeGroup, Name: f.groups[roomID].Name})
		}
	}
	for roomID, fr := range f.friends {
		if fr.UID1 == uid || fr.UID2 == uid {
			peer := fr.UID2
			if fr.UID1 == uid {
				peer = fr.UID2
			} else {
				peer = fr.UID1
			}
			out = append(out, RoomSummary{RoomID: roomID, Type: TypeFriend, PeerUID: peer})
		}
	}
	return out, nil
}

func (f *fakeStore) GetOrCreateFriendRoom(ctx context.Context, uid1, uid2 int64) (int64, error) {
	for roomID, fr := range f.friends {
		if (fr.UID1 == uid1 && fr.UID2 == uid2) || (fr.UID1 == uid2 && fr.UID2 == uid1) {
			return roomID, nil
		}
	}
	f.nextID++
	roomID := f.nextID
	f.rooms[roomID] = &Room{ID: roomID, Type: TypeFriend}
	f.friends[roomID] = &Friend{ID: roomID, RoomID: roomID, UID1: uid1, UID2: uid2}
	return roomID, nil
}

type fakeMemberCache struct {
	invalidated []int64
}

func (c *fakeMemberCache) Invalidate(ctx context.Context, groupID int64) error {
	c.invalidated = append(c.invalidated, groupID)
	return nil
}

func TestCreateGroupAndAddMember(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	cache := &fakeMemberCache{}
	h := NewHandler(store, cache)
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
	if len(cache.invalidated) != 1 || cache.invalidated[0] != roomID {
		t.Fatalf("expected addMember to invalidate the cache for group %d, got %v", roomID, cache.invalidated)
	}
}

func TestRemoveMemberForbiddenForRegularMember(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	roomID, _ := store.CreateGroupRoom(context.Background(), 1, "g", "")
	store.AddMember(context.Background(), roomID, 2, RoleMember)
	store.AddMember(context.Background(), roomID, 3, RoleMember)

	h := NewHandler(store, nil)
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

	h := NewHandler(store, nil)
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

func TestCreateFriendRoomThenListRooms(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	h := NewHandler(store, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token1, _ := auth.GenerateToken(1, secret, 3600_000_000_000)
	body, _ := json.Marshal(map[string]int64{"uid": 2})
	req := httptest.NewRequest(http.MethodPost, "/api/rooms/friends", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token1)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("createFriendRoom status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created map[string]int64
	json.NewDecoder(rec.Body).Decode(&created)
	roomID := created["room_id"]

	// Calling it again for the same pair must return the same room, not create a duplicate.
	req = httptest.NewRequest(http.MethodPost, "/api/rooms/friends", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token1)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	var createdAgain map[string]int64
	json.NewDecoder(rec.Body).Decode(&createdAgain)
	if createdAgain["room_id"] != roomID {
		t.Fatalf("second call created a new room %d, want reuse of %d", createdAgain["room_id"], roomID)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("listRooms status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var rooms []RoomSummary
	json.NewDecoder(rec.Body).Decode(&rooms)
	found := false
	for _, rm := range rooms {
		if rm.RoomID == roomID && rm.Type == TypeFriend && rm.PeerUID == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the DM room with peer_uid=2 in list, got %+v", rooms)
	}
}

func TestCreateFriendRoomRejectsSelf(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	h := NewHandler(store, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(1, secret, 3600_000_000_000)
	body, _ := json.Marshal(map[string]int64{"uid": 1})
	req := httptest.NewRequest(http.MethodPost, "/api/rooms/friends", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSetAdminForbiddenForNonOwner(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	roomID, _ := store.CreateGroupRoom(context.Background(), 1, "g", "")
	store.AddMember(context.Background(), roomID, 2, RoleMember)
	store.AddMember(context.Background(), roomID, 3, RoleMember)

	h := NewHandler(store, nil)
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
