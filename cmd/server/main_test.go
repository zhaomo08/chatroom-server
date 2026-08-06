package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"chatroom-server/internal/config"
)

func TestHealthz(t *testing.T) {
	dsn := os.Getenv("CHATROOM_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CHATROOM_TEST_MYSQL_DSN not set, skipping (buildMux needs a live MySQL/Redis to wire up)")
	}
	cfg := &config.Config{MySQLDSN: dsn, RedisAddr: os.Getenv("CHATROOM_TEST_REDIS_ADDR"), JWTSecret: "test-secret"}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	buildMux(cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
