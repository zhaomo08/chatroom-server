package auth

import (
	"context"
	"os"
	"testing"

	"chatroom-server/internal/db"
)

func TestSQLStoreGetUsersByIDs(t *testing.T) {
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

	hash, _ := HashPassword("s3cret!")
	id1, err := store.CreateUser(ctx, "lookup_a", hash, "Alice")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	id2, err := store.CreateUser(ctx, "lookup_b", hash, "Bob")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	users, err := store.GetUsersByIDs(ctx, []int64{id1, id2, 987654321})
	if err != nil {
		t.Fatalf("GetUsersByIDs: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2 (nonexistent id silently omitted)", len(users))
	}
	names := map[int64]string{}
	for _, u := range users {
		names[u.ID] = u.Nickname
	}
	if names[id1] != "Alice" || names[id2] != "Bob" {
		t.Errorf("names = %+v, want Alice/Bob", names)
	}
}

func TestSQLStoreGetUsersByIDsEmpty(t *testing.T) {
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
	users, err := store.GetUsersByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetUsersByIDs: %v", err)
	}
	if users == nil {
		t.Error("GetUsersByIDs(nil) should return a non-nil empty slice, not nil (would serialize as JSON null)")
	}
}
