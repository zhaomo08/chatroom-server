package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/redis/go-redis/v9"

	"chatroom-server/internal/auth"
	"chatroom-server/internal/cache"
	"chatroom-server/internal/config"
	"chatroom-server/internal/db"
	"chatroom-server/internal/message"
	"chatroom-server/internal/room"
	"chatroom-server/internal/ws"
)

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func buildMux(cfg *config.Config) *http.ServeMux {
	sqlDB, err := db.Connect(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("connect mysql: %v", err)
	}
	if err := db.Migrate(cfg.MySQLDSN); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPass})
	// Wired for future use by room/message services that want cached member
	// lookups instead of hitting MySQL on every send; not consumed yet.
	_ = cache.New(rdb)

	authStore := auth.NewSQLStore(sqlDB)
	roomStore := room.NewSQLStore(sqlDB)
	msgStore := message.NewSQLStore(sqlDB)
	hub := ws.NewHub()

	authHandler := auth.NewHandler(authStore, []byte(cfg.JWTSecret), cfg.JWTTTL)
	roomHandler := room.NewHandler(roomStore)
	msgHandler := message.NewHandler(msgStore, roomStore, hub)
	wsHandler := ws.NewHandler(hub, []byte(cfg.JWTSecret))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	authHandler.Register(mux)
	wsHandler.RegisterRoutes(mux)

	protected := http.NewServeMux()
	roomHandler.RegisterRoutes(protected)
	msgHandler.RegisterRoutes(protected)
	authMiddleware := auth.Middleware([]byte(cfg.JWTSecret))
	mux.Handle("/api/rooms", authMiddleware(protected))
	mux.Handle("/api/rooms/", authMiddleware(protected))
	mux.Handle("/api/messages", authMiddleware(protected))
	mux.Handle("/api/messages/", authMiddleware(protected))

	return mux
}

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	mux := buildMux(cfg)
	log.Printf("chatroom-server listening on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, mux); err != nil {
		log.Fatal(err)
	}
}
