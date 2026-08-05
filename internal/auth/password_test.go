package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword(hash, "s3cret!") {
		t.Error("CheckPassword should succeed with correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("CheckPassword should fail with wrong password")
	}
}
