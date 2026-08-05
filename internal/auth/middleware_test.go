package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddlewareValidToken(t *testing.T) {
	secret := []byte("test-secret")
	token, _ := GenerateToken(7, secret, time.Hour)

	var gotUID int64
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUID, _ = UIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	Middleware(secret)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUID != 7 {
		t.Errorf("uid in context = %d, want 7", gotUID)
	}
}

func TestMiddlewareMissingOrBadToken(t *testing.T) {
	secret := []byte("test-secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, header := range []string{"", "Bearer not-a-token", "NoBearerPrefix xyz"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()

		Middleware(secret)(next).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", header, rec.Code)
		}
	}
}
