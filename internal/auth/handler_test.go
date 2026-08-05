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

func (f *fakeStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	u, ok := f.users[username]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return u, nil
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
