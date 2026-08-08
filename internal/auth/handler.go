package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Handler struct {
	store  Store
	secret []byte
	ttl    time.Duration
}

func NewHandler(store Store, secret []byte, ttl time.Duration) *Handler {
	return &Handler{store: store, secret: secret, ttl: ttl}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/register", h.handleRegister)
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
}

// RegisterProtected registers routes that need an authenticated caller.
// Mounted behind the Bearer-header middleware, unlike Register above.
func (h *Handler) RegisterProtected(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/users/lookup", h.handleLookup)
	mux.HandleFunc("PUT /api/users/me", h.handleUpdateProfile)
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Username) < 3 || len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "username must be >=3 chars and password >=6 chars")
		return
	}
	if req.Nickname == "" {
		req.Nickname = req.Username
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	uid, err := h.store.CreateUser(ctx, req.Username, hash, req.Nickname)
	if err != nil {
		writeError(w, http.StatusConflict, "username already taken")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"uid": uid})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, err := h.store.GetUserByUsername(ctx, req.Username)
	if err != nil || !CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, err := GenerateToken(user.ID, h.secret, h.ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

type lookupRequest struct {
	UIDs []int64 `json:"uids"`
}

type userInfo struct {
	UID      int64  `json:"uid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request) {
	var req lookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.UIDs) == 0 || len(req.UIDs) > 200 {
		writeError(w, http.StatusBadRequest, "uids must contain 1-200 entries")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	users, err := h.store.GetUsersByIDs(ctx, req.UIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up users")
		return
	}
	out := make([]userInfo, len(users))
	for i, u := range users {
		out[i] = userInfo{UID: u.ID, Nickname: u.Nickname, Avatar: u.Avatar}
	}
	writeJSON(w, http.StatusOK, out)
}

type updateProfileRequest struct {
	Nickname *string `json:"nickname"`
	Avatar   *string `json:"avatar"`
}

// handleUpdateProfile supports partial updates (nickname-only or
// avatar-only) by reading the caller's current row and only overwriting the
// fields present in the request, rather than requiring the client to
// resend both every time.
func (h *Handler) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	uid, _ := UIDFromContext(r.Context())

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Nickname != nil && (len(*req.Nickname) == 0 || len(*req.Nickname) > 32) {
		writeError(w, http.StatusBadRequest, "nickname must be 1-32 characters")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	current, err := h.store.GetUsersByIDs(ctx, []int64{uid})
	if err != nil || len(current) == 0 {
		writeError(w, http.StatusInternalServerError, "failed to load current profile")
		return
	}
	nickname, avatar := current[0].Nickname, current[0].Avatar
	if req.Nickname != nil {
		nickname = *req.Nickname
	}
	if req.Avatar != nil {
		avatar = *req.Avatar
	}

	if err := h.store.UpdateProfile(ctx, uid, nickname, avatar); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	writeJSON(w, http.StatusOK, userInfo{UID: uid, Nickname: nickname, Avatar: avatar})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status, "msg": msg})
}
