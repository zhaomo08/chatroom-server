package room

import (
	"context"
	"os"
	"testing"

	"chatroom-server/internal/db"
)

func TestSQLStoreCreateGroupAndMembers(t *testing.T) {
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

	roomID, err := store.CreateGroupRoom(ctx, 1, "test group", "")
	if err != nil {
		t.Fatalf("CreateGroupRoom: %v", err)
	}

	if err := store.AddMember(ctx, roomID, 2, RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	members, err := store.ListMembers(ctx, roomID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(members))
	}

	if err := store.RemoveMember(ctx, roomID, 2); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if _, err := store.GetMember(ctx, roomID, 2); err != ErrNotFound {
		t.Errorf("GetMember after removal = %v, want ErrNotFound", err)
	}
}
