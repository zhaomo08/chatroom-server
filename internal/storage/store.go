package storage

import (
	"context"
	"io"
)

// Store persists opaque binary objects (images/videos) addressed by key.
// Get returns an io.ReadSeekCloser so callers can hand it to
// http.ServeContent and get Range-request support (video scrubbing) for free.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (obj io.ReadSeekCloser, size int64, contentType string, err error)
}
