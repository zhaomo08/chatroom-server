package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"time"

	"golang.org/x/image/draw"

	"chatroom-server/internal/auth"
)

const (
	maxImageSize = 10 << 20 // 10MB
	maxVideoSize = 50 << 20 // 50MB
	thumbMaxDim  = 320
)

var allowedImageMime = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

var allowedVideoMime = map[string]bool{
	"video/mp4":  true,
	"video/webm": true,
}

// Store is the subset of storage.Store this package depends on, kept as a
// local interface so tests can use an in-memory fake without importing MinIO.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (obj io.ReadSeekCloser, size int64, contentType string, err error)
}

type Handler struct {
	store  Store
	secret []byte
}

func NewHandler(store Store, secret []byte) *Handler {
	return &Handler{store: store, secret: secret}
}

// RegisterRoutes registers the upload endpoint. Callers should mount this on
// a mux that sits behind the standard Bearer-header auth middleware.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/media/upload", h.upload)
}

// RegisterPublicRoutes registers the download endpoint on a mux that is NOT
// behind the Bearer-header middleware: <img>/<video> tags can't send custom
// headers, so this endpoint checks a ?token= query param instead (the same
// pattern the WebSocket handshake already uses).
func (h *Handler) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/{id}", h.download)
}

type uploadResponse struct {
	FileID  string `json:"file_id"`
	ThumbID string `json:"thumb_id,omitempty"`
	Mime    string `json:"mime"`
	Size    int64  `json:"size"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxVideoSize + (1 << 20)); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse upload (max 50MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(header.Filename)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	switch {
	case allowedImageMime[contentType]:
		h.uploadImage(ctx, w, file, header.Size, contentType)
	case allowedVideoMime[contentType]:
		h.uploadVideo(ctx, w, file, header.Size, contentType)
	default:
		writeError(w, http.StatusBadRequest, "unsupported file type: "+contentType)
	}
}

func (h *Handler) uploadImage(ctx context.Context, w http.ResponseWriter, file io.Reader, size int64, contentType string) {
	if size > maxImageSize {
		writeError(w, http.StatusBadRequest, "image too large (max 10MB)")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxImageSize+1))
	if err != nil || int64(len(data)) > maxImageSize {
		writeError(w, http.StatusBadRequest, "failed to read image")
		return
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image data")
		return
	}
	bounds := img.Bounds()

	fileID := newID()
	if err := h.store.Put(ctx, fileID, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store image")
		return
	}

	thumbID := newID()
	thumbData := makeThumbnail(img)
	if err := h.store.Put(ctx, thumbID, bytes.NewReader(thumbData), int64(len(thumbData)), "image/jpeg"); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store thumbnail")
		return
	}

	writeJSON(w, http.StatusOK, uploadResponse{
		FileID: fileID, ThumbID: thumbID, Mime: contentType, Size: int64(len(data)),
		Width: bounds.Dx(), Height: bounds.Dy(),
	})
}

func (h *Handler) uploadVideo(ctx context.Context, w http.ResponseWriter, file io.Reader, size int64, contentType string) {
	if size > maxVideoSize {
		writeError(w, http.StatusBadRequest, "video too large (max 50MB)")
		return
	}
	fileID := newID()
	if err := h.store.Put(ctx, fileID, io.LimitReader(file, maxVideoSize), size, contentType); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store video")
		return
	}
	writeJSON(w, http.StatusOK, uploadResponse{FileID: fileID, Mime: contentType, Size: size})
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	if _, err := auth.ParseToken(r.URL.Query().Get("token"), h.secret); err != nil {
		http.Error(w, `{"code":401,"msg":"invalid token"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	obj, _, contentType, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer obj.Close()

	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, id, time.Time{}, obj)
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck // crypto/rand.Read never returns an error on supported platforms
	return hex.EncodeToString(b)
}

func makeThumbnail(img image.Image) []byte {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	longEdge := w
	if h > longEdge {
		longEdge = h
	}
	scale := float64(thumbMaxDim) / float64(longEdge)
	if scale > 1 {
		scale = 1
	}
	dstW, dstH := int(float64(w)*scale), int(float64(h)*scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}) //nolint:errcheck // encoding an in-memory RGBA to a bytes.Buffer cannot fail
	return buf.Bytes()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status, "msg": msg})
}
