package message

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"chatroom-server/internal/auth"
	"chatroom-server/internal/room"
)

type fakeMsgStore struct {
	msgs   map[int64]*Message
	nextID int64
	marks  []Mark
}

func newFakeMsgStore() *fakeMsgStore { return &fakeMsgStore{msgs: map[int64]*Message{}} }

func (f *fakeMsgStore) Insert(ctx context.Context, m *Message) (int64, error) {
	f.nextID++
	m.ID = f.nextID
	m.CreateTime = time.Now()
	f.msgs[m.ID] = m
	return m.ID, nil
}

func (f *fakeMsgStore) GetByID(ctx context.Context, id int64) (*Message, error) {
	m, ok := f.msgs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return m, nil
}

func (f *fakeMsgStore) SetStatus(ctx context.Context, id int64, status Status) error {
	f.msgs[id].Status = status
	return nil
}

func (f *fakeMsgStore) AddMark(ctx context.Context, msgID, uid int64, markType int) error {
	f.marks = append(f.marks, Mark{MsgID: msgID, UID: uid, Type: markType})
	return nil
}

func (f *fakeMsgStore) ListByRoomCursor(ctx context.Context, roomID, beforeID int64, limit int) ([]Message, error) {
	var out []Message
	for _, m := range f.msgs {
		if m.RoomID == roomID {
			out = append(out, *m)
		}
	}
	return out, nil
}

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

type fakeHub struct {
	sentTo    map[int64][][]byte
	broadcast [][]byte
}

func newFakeHub() *fakeHub { return &fakeHub{sentTo: map[int64][][]byte{}} }

func (h *fakeHub) SendToUsers(uids []int64, payload []byte) {
	for _, uid := range uids {
		h.sentTo[uid] = append(h.sentTo[uid], payload)
	}
}

func (h *fakeHub) BroadcastAll(payload []byte) {
	h.broadcast = append(h.broadcast, payload)
}

func TestSendMessageToGroupMembers(t *testing.T) {
	secret := []byte("test-secret")
	msgStore := newFakeMsgStore()
	roomStore := &fakeRoomStore{
		rooms:   map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {{GroupID: 10, UID: 1, Role: room.RoleOwner}, {GroupID: 10, UID: 2, Role: room.RoleMember}}},
	}
	hub := newFakeHub()
	h := NewHandler(msgStore, roomStore, hub)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	body, _ := json.Marshal(map[string]any{"room_id": 10, "content": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(hub.sentTo[1]) != 1 || len(hub.sentTo[2]) != 1 {
		t.Fatalf("expected both members to receive the broadcast, got sentTo = %+v", hub.sentTo)
	}
}

func TestSendForbiddenForNonMember(t *testing.T) {
	secret := []byte("test-secret")
	msgStore := newFakeMsgStore()
	roomStore := &fakeRoomStore{
		rooms:   map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {{GroupID: 10, UID: 1, Role: room.RoleOwner}}},
	}
	h := NewHandler(msgStore, roomStore, newFakeHub())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	// uid 2 is not a member of room 10.
	token, _ := auth.GenerateToken(2, secret, time.Hour)
	body, _ := json.Marshal(map[string]any{"room_id": 10, "content": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSendMessageToHotRoomBroadcastsAll(t *testing.T) {
	secret := []byte("test-secret")
	msgStore := newFakeMsgStore()
	roomStore := &fakeRoomStore{
		rooms: map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup, HotFlag: room.HotYes}},
	}
	hub := newFakeHub()
	h := NewHandler(msgStore, roomStore, hub)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	body, _ := json.Marshal(map[string]any{"room_id": 10, "content": "hello everyone"})
	req := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(hub.broadcast) != 1 {
		t.Fatalf("expected one BroadcastAll call for a hot room, got %d", len(hub.broadcast))
	}
}

func TestRecallOwnMessageWithinWindow(t *testing.T) {
	secret := []byte("test-secret")
	msgStore := newFakeMsgStore()
	msgStore.msgs[1] = &Message{ID: 1, RoomID: 10, FromUID: 1, CreateTime: time.Now()}
	roomStore := &fakeRoomStore{
		rooms:   map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {{GroupID: 10, UID: 1, Role: room.RoleMember}}},
	}
	h := NewHandler(msgStore, roomStore, newFakeHub())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	req := httptest.NewRequest(http.MethodPut, "/api/messages/"+strconv.Itoa(1)+"/recall", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if msgStore.msgs[1].Status != StatusDeleted {
		t.Error("message should be marked deleted after recall")
	}
}

func TestRecallForbiddenForOtherMember(t *testing.T) {
	secret := []byte("test-secret")
	msgStore := newFakeMsgStore()
	msgStore.msgs[1] = &Message{ID: 1, RoomID: 10, FromUID: 1, CreateTime: time.Now()}
	roomStore := &fakeRoomStore{
		rooms:   map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}},
		members: map[int64][]room.Member{10: {{GroupID: 10, UID: 1, Role: room.RoleMember}, {GroupID: 10, UID: 2, Role: room.RoleMember}}},
	}
	h := NewHandler(msgStore, roomStore, newFakeHub())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(2, secret, time.Hour)
	req := httptest.NewRequest(http.MethodPut, "/api/messages/1/recall", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestPageListsMessages(t *testing.T) {
	secret := []byte("test-secret")
	msgStore := newFakeMsgStore()
	msgStore.Insert(context.Background(), &Message{RoomID: 10, FromUID: 1, Content: "a", Type: TypeText})
	msgStore.Insert(context.Background(), &Message{RoomID: 10, FromUID: 1, Content: "b", Type: TypeText})
	roomStore := &fakeRoomStore{rooms: map[int64]*room.Room{10: {ID: 10, Type: room.TypeGroup}}}
	h := NewHandler(msgStore, roomStore, newFakeHub())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	wrapped := auth.Middleware(secret)(mux)

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/api/rooms/10/messages", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var msgs []Message
	json.NewDecoder(rec.Body).Decode(&msgs)
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
}
