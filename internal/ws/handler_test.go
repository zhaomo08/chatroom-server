package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"chatroom-server/internal/auth"
)

func TestServeWSDeliversBroadcast(t *testing.T) {
	secret := []byte("test-secret")
	hub := NewHub()
	h := NewHandler(hub, secret)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + token

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Give the server a moment to finish registering the connection in the Hub.
	time.Sleep(50 * time.Millisecond)

	hub.SendToUsers([]int64{1}, []byte("hello"))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("data = %q, want %q", data, "hello")
	}
}

func TestServeWSRejectsInvalidToken(t *testing.T) {
	secret := []byte("test-secret")
	hub := NewHub()
	h := NewHandler(hub, secret)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=not-a-real-token"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected Dial to fail for an invalid token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Errorf("status = %d, want 401", status)
	}
}
