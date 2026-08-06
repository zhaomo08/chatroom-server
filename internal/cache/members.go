package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type MemberCache struct{ rdb *redis.Client }

func New(rdb *redis.Client) *MemberCache { return &MemberCache{rdb: rdb} }

func key(groupID int64) string {
	return fmt.Sprintf("chatroom:group_members:%d", groupID)
}

func (c *MemberCache) Get(ctx context.Context, groupID int64) ([]int64, bool, error) {
	data, err := c.rdb.Get(ctx, key(groupID)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var uids []int64
	if err := json.Unmarshal(data, &uids); err != nil {
		return nil, false, err
	}
	return uids, true, nil
}

func (c *MemberCache) Set(ctx context.Context, groupID int64, uids []int64, ttl time.Duration) error {
	data, err := json.Marshal(uids)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key(groupID), data, ttl).Err()
}

func (c *MemberCache) Invalidate(ctx context.Context, groupID int64) error {
	return c.rdb.Del(ctx, key(groupID)).Err()
}
