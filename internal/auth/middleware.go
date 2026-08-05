package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey string

const uidKey ctxKey = "uid"

func Middleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(header, prefix) {
				http.Error(w, `{"code":401,"msg":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}
			uid, err := ParseToken(strings.TrimPrefix(header, prefix), secret)
			if err != nil {
				http.Error(w, `{"code":401,"msg":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), uidKey, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UIDFromContext(ctx context.Context) (int64, bool) {
	uid, ok := ctx.Value(uidKey).(int64)
	return uid, ok
}
