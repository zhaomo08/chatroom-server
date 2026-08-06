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

func TestSQLStoreListRoomsAndFriendRoom(t *testing.T) {
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

	groupRoomID, err := store.CreateGroupRoom(ctx, 101, "team room", "")
	if err != nil {
		t.Fatalf("CreateGroupRoom: %v", err)
	}

	friendRoomID, err := store.GetOrCreateFriendRoom(ctx, 101, 102)
	if err != nil {
		t.Fatalf("GetOrCreateFriendRoom: %v", err)
	}
	// Calling it again (with args swapped) must return the same room, not create a duplicate.
	againRoomID, err := store.GetOrCreateFriendRoom(ctx, 102, 101)
	if err != nil {
		t.Fatalf("GetOrCreateFriendRoom (again): %v", err)
	}
	if againRoomID != friendRoomID {
		t.Fatalf("GetOrCreateFriendRoom returned a different room on second call: %d vs %d", againRoomID, friendRoomID)
	}

	rooms, err := store.ListRoomsForUser(ctx, 101)
	if err != nil {
		t.Fatalf("ListRoomsForUser: %v", err)
	}
	var sawGroup, sawFriend bool
	for _, rm := range rooms {
		if rm.RoomID == groupRoomID && rm.Type == TypeGroup {
			sawGroup = true
		}
		if rm.RoomID == friendRoomID && rm.Type == TypeFriend && rm.PeerUID == 102 {
			sawFriend = true
		}
	}
	if !sawGroup || !sawFriend {
		t.Fatalf("ListRoomsForUser missing expected rooms, got %+v", rooms)
	}
}
