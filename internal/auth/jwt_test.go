package auth

import (
	"testing"
	"time"
)

func TestGenerateAndParseToken(t *testing.T) {
	secret := []byte("test-secret")

	token, err := GenerateToken(42, secret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	uid, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if uid != 42 {
		t.Errorf("uid = %d, want 42", uid)
	}
}

func TestParseTokenExpired(t *testing.T) {
	secret := []byte("test-secret")

	token, err := GenerateToken(42, secret, -time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	if _, err := ParseToken(token, secret); err == nil {
		t.Error("ParseToken should fail for an expired token")
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	token, err := GenerateToken(42, []byte("secret-a"), time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	if _, err := ParseToken(token, []byte("secret-b")); err == nil {
		t.Error("ParseToken should fail when secret does not match")
	}
}
