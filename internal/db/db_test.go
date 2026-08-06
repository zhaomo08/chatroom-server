package db

import (
	"os"
	"testing"
)

func TestConnectAndMigrate(t *testing.T) {
	dsn := os.Getenv("CHATROOM_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CHATROOM_TEST_MYSQL_DSN not set, skipping integration test (start docker-compose mysql to run this)")
	}

	if err := Migrate(dsn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	conn, err := Connect(dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
