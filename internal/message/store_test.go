package message

import (
	"context"
	"os"
	"testing"

	"chatroom-server/internal/db"
)

func TestSQLStoreInsertAndPage(t *testing.T) {
	dsn := os.Getenv("CHATROOM_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CHATROOM_TEST_MYSQL_DSN not set, skipping integration test")
	}

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	conn, err := db.Connect(dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	store := NewSQLStore(conn)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := store.Insert(ctx, &Message{RoomID: 999, FromUID: 1, Content: "hi", Type: TypeText}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	page, err := store.ListByRoomCursor(ctx, 999, 0, 2)
	if err != nil {
		t.Fatalf("ListByRoomCursor: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("len(page) = %d, want 2", len(page))
	}
}
