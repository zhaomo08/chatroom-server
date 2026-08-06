package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestClient(t *testing.T) *redis.Client {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestMemberCacheSetGetInvalidate(t *testing.T) {
	rdb := newTestClient(t)
	c := New(rdb)
	ctx := context.Background()

	if _, ok, err := c.Get(ctx, 42); err != nil || ok {
		t.Fatalf("expected cache miss before Set, ok=%v err=%v", ok, err)
	}

	if err := c.Set(ctx, 42, []int64{1, 2, 3}, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	uids, ok, err := c.Get(ctx, 42)
	if err != nil || !ok {
		t.Fatalf("expected cache hit, ok=%v err=%v", ok, err)
	}
	if len(uids) != 3 {
		t.Fatalf("uids = %v, want 3 elements", uids)
	}

	if err := c.Invalidate(ctx, 42); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	if _, ok, _ := c.Get(ctx, 42); ok {
		t.Error("expected cache miss after Invalidate")
	}
}
