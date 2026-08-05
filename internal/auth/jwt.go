package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UID int64 `json:"uid"`
	jwt.RegisteredClaims
}

func GenerateToken(uid int64, secret []byte, ttl time.Duration) (string, error) {
	claims := Claims{
		UID: uid,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func ParseToken(tokenStr string, secret []byte) (int64, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil {
		return 0, err
	}
	if !token.Valid {
		return 0, errors.New("invalid token")
	}
	return claims.UID, nil
}
