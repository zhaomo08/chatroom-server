package media

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatroom-server/internal/auth"
)

type fakeStore struct {
	objects map[string][]byte
	mimes   map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{objects: map[string][]byte{}, mimes: map[string]string{}}
}

func (f *fakeStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = data
	f.mimes[key] = contentType
	return nil
}

type readSeekNopCloser struct{ *bytes.Reader }

func (readSeekNopCloser) Close() error { return nil }

func (f *fakeStore) Get(ctx context.Context, key string) (io.ReadSeekCloser, int64, string, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, 0, "", errNotFound
	}
	return readSeekNopCloser{bytes.NewReader(data)}, int64(len(data)), f.mimes[key], nil
}

var errNotFound = &notFoundError{}

type notFoundError struct{}

func (*notFoundError) Error() string { return "not found" }

func multipartImageBody(t *testing.T, filename, contentType string, data []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func fourByFourPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestUploadImageGeneratesThumbnail(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	h := NewHandler(store, secret)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, contentType := multipartImageBody(t, "pic.png", "image/png", fourByFourPNG(t))
	req := httptest.NewRequest(http.MethodPost, "/api/media/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp uploadResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.FileID == "" || resp.ThumbID == "" {
		t.Fatalf("expected non-empty file_id and thumb_id, got %+v", resp)
	}
	if resp.Width != 4 || resp.Height != 4 {
		t.Errorf("dimensions = %dx%d, want 4x4", resp.Width, resp.Height)
	}
	if _, ok := store.objects[resp.FileID]; !ok {
		t.Error("original image was not stored")
	}
	if _, ok := store.objects[resp.ThumbID]; !ok {
		t.Error("thumbnail was not stored")
	}
}

func TestUploadRejectsUnsupportedType(t *testing.T) {
	secret := []byte("test-secret")
	h := NewHandler(newFakeStore(), secret)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, contentType := multipartImageBody(t, "notes.txt", "text/plain", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/api/media/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUploadRejectsOversizedImage(t *testing.T) {
	secret := []byte("test-secret")
	h := NewHandler(newFakeStore(), secret)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	huge := make([]byte, maxImageSize+1)
	body, contentType := multipartImageBody(t, "huge.jpg", "image/jpeg", huge)
	req := httptest.NewRequest(http.MethodPost, "/api/media/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestDownloadRequiresValidToken(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	store.objects["abc"] = []byte("data")
	store.mimes["abc"] = "image/png"
	h := NewHandler(store, secret)
	mux := http.NewServeMux()
	h.RegisterPublicRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/media/abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestDownloadServesStoredContent(t *testing.T) {
	secret := []byte("test-secret")
	store := newFakeStore()
	store.objects["abc"] = []byte("hello media bytes")
	store.mimes["abc"] = "image/png"
	h := NewHandler(store, secret)
	mux := http.NewServeMux()
	h.RegisterPublicRoutes(mux)

	token, _ := auth.GenerateToken(1, secret, time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/api/media/abc?token="+token, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "hello media bytes" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "hello media bytes")
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", rec.Header().Get("Content-Type"))
	}
}
