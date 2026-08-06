package call

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMintTokenClaims(t *testing.T) {
	tok, err := mintToken("devkey", "devsecret", "42", "room-7", time.Hour)
	if err != nil {
		t.Fatalf("mintToken: %v", err)
	}

	claims := &accessTokenClaims{}
	parsed, err := jwt.ParseWithClaims(tok, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte("devsecret"), nil
	})
	if err != nil {
		t.Fatalf("ParseWithClaims: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("token should be valid")
	}
	if claims.Issuer != "devkey" {
		t.Errorf("Issuer = %q, want devkey", claims.Issuer)
	}
	if claims.Subject != "42" {
		t.Errorf("Subject = %q, want 42", claims.Subject)
	}
	if claims.Video.Room != "room-7" {
		t.Errorf("Video.Room = %q, want room-7", claims.Video.Room)
	}
	if !claims.Video.RoomJoin || !claims.Video.CanPublish || !claims.Video.CanSubscribe {
		t.Errorf("Video grant = %+v, want all true", claims.Video)
	}
}

func TestMintTokenWrongSecretRejected(t *testing.T) {
	tok, err := mintToken("devkey", "devsecret", "42", "room-7", time.Hour)
	if err != nil {
		t.Fatalf("mintToken: %v", err)
	}

	claims := &accessTokenClaims{}
	_, err = jwt.ParseWithClaims(tok, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte("wrong-secret"), nil
	})
	if err == nil {
		t.Fatal("expected parsing to fail with the wrong secret")
	}
}
