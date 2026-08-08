package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeStore struct {
	users map[string]*User
	next  int64
}

func newFakeStore() *fakeStore { return &fakeStore{users: map[string]*User{}} }

func (f *fakeStore) CreateUser(ctx context.Context, username, passwordHash, nickname string) (int64, error) {
	if _, exists := f.users[username]; exists {
		return 0, sql.ErrNoRows // reuse a stdlib error to simulate a duplicate-key failure
	}
	f.next++
	f.users[username] = &User{ID: f.next, Username: username, PasswordHash: passwordHash, Nickname: nickname}
	return f.next, nil
}

func (f *fakeStore) GetUsersByIDs(ctx context.Context, ids []int64) ([]User, error) {
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []User
	for _, u := range f.users {
		if want[u.ID] {
			out = append(out, *u)
		}
	}
	return out, nil
}

func (f *fakeStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	u, ok := f.users[username]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return u, nil
}

func (f *fakeStore) UpdateProfile(ctx context.Context, uid int64, nickname, avatar string) error {
	for _, u := range f.users {
		if u.ID == uid {
			u.Nickname, u.Avatar = nickname, avatar
			return nil
		}
	}
	return sql.ErrNoRows
}

func TestHandlerRegisterAndLogin(t *testing.T) {
	h := NewHandler(newFakeStore(), []byte("test-secret"), time.Hour)
	mux := http.NewServeMux()
	h.Register(mux)

	body, _ := json.Marshal(map[string]string{"username": "alice", "password": "s3cret!", "nickname": "Alice"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body.String())
	}

	body, _ = json.Marshal(map[string]string{"username": "alice", "password": "s3cret!"})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestHandlerLoginWrongPassword(t *testing.T) {
	store := newFakeStore()
	h := NewHandler(store, []byte("test-secret"), time.Hour)
	mux := http.NewServeMux()
	h.Register(mux)

	hash, _ := HashPassword("correct")
	store.users["bob"] = &User{ID: 1, Username: "bob", PasswordHash: hash}

	body, _ := json.Marshal(map[string]string{"username": "bob", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandlerLookup(t *testing.T) {
	store := newFakeStore()
	store.users["alice"] = &User{ID: 1, Username: "alice", Nickname: "Alice", Avatar: "a.png"}
	store.users["bob"] = &User{ID: 2, Username: "bob", Nickname: "Bob"}
	h := NewHandler(store, []byte("test-secret"), time.Hour)
	mux := http.NewServeMux()
	h.RegisterProtected(mux)

	body, _ := json.Marshal(map[string]any{"uids": []int64{1, 2, 999}})
	req := httptest.NewRequest(http.MethodPost, "/api/users/lookup", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var users []userInfo
	if err := json.NewDecoder(rec.Body).Decode(&users); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2 (unknown uid 999 silently omitted)", len(users))
	}
}

func TestHandlerLookupRejectsEmpty(t *testing.T) {
	h := NewHandler(newFakeStore(), []byte("test-secret"), time.Hour)
	mux := http.NewServeMux()
	h.RegisterProtected(mux)

	body, _ := json.Marshal(map[string]any{"uids": []int64{}})
	req := httptest.NewRequest(http.MethodPost, "/api/users/lookup", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerUpdateProfilePartial(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	store.users["alice"] = &User{ID: 1, Username: "alice", Nickname: "Alice", Avatar: "old.png"}
	h := NewHandler(store, secret, time.Hour)
	mux := http.NewServeMux()
	h.RegisterProtected(mux)
	wrapped := Middleware(secret)(mux)

	token, _ := GenerateToken(1, secret, time.Hour)

	// Nickname-only update must leave the existing avatar untouched.
	body, _ := json.Marshal(map[string]any{"nickname": "Alicia"})
	req := httptest.NewRequest(http.MethodPut, "/api/users/me", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp userInfo
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Nickname != "Alicia" || resp.Avatar != "old.png" {
		t.Errorf("resp = %+v, want nickname=Alicia avatar=old.png", resp)
	}
	if store.users["alice"].Nickname != "Alicia" || store.users["alice"].Avatar != "old.png" {
		t.Errorf("stored user = %+v, want nickname=Alicia avatar=old.png", store.users["alice"])
	}
}

func TestHandlerUpdateProfileRejectsEmptyNickname(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	store.users["alice"] = &User{ID: 1, Username: "alice", Nickname: "Alice"}
	h := NewHandler(store, secret, time.Hour)
	mux := http.NewServeMux()
	h.RegisterProtected(mux)
	wrapped := Middleware(secret)(mux)

	token, _ := GenerateToken(1, secret, time.Hour)
	body, _ := json.Marshal(map[string]any{"nickname": ""})
	req := httptest.NewRequest(http.MethodPut, "/api/users/me", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
