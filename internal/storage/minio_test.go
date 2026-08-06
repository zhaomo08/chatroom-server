package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

func TestMinioStorePutAndGet(t *testing.T) {
	endpoint := os.Getenv("CHATROOM_TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("CHATROOM_TEST_MINIO_ENDPOINT not set, skipping integration test")
	}

	store, err := NewMinioStore(context.Background(), endpoint, "minioadmin", "minioadmin", "chatroom-media-test", false)
	if err != nil {
		t.Fatalf("NewMinioStore: %v", err)
	}

	data := []byte("hello media")
	if err := store.Put(context.Background(), "test-key", bytes.NewReader(data), int64(len(data)), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	obj, size, contentType, err := store.Get(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer obj.Close()

	if size != int64(len(data)) {
		t.Errorf("size = %d, want %d", size, len(data))
	}
	if contentType != "text/plain" {
		t.Errorf("contentType = %q, want text/plain", contentType)
	}

	got, err := io.ReadAll(obj)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("data = %q, want %q", got, data)
	}
}
