package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"trpc.group/trpc-go/trpc-database/goredis"
	"trpc.group/trpc-go/trpc-go/log"
)

const (
	// negCacheValue 是空值缓存的标记字符串
	negCacheValue = "__NULL__"

	// 命中数据的基准 TTL：30 分钟，再叠加 0~5 分钟随机抖动防雪崩。
	cacheTTLBase   = 30 * time.Minute
	cacheTTLJitter = 5 * time.Minute

	// 空值缓存只缓存 60s
	negCacheTTL = 60 * time.Second
)

var (
	// ErrCacheMiss：cache 中没有数据，应回查 DB。
	ErrCacheMiss = errors.New("cache miss")

	// ErrCacheNegative：命中空值缓存。表示 DB 中确实不存在，
	ErrCacheNegative = errors.New("cache negative hit")
)

type RedisCache struct {
	client redis.UniversalClient
}

// NewRedisCache 入参 name 必须与 trpc_go.yaml 中 client.service[].name 完全一致。
func NewRedisCache(name string) *RedisCache {
	cli, err := goredis.New(name)
	if err != nil {
		log.Fatalf("goredis.New(%s) fail: %v", name, err)
	}
	return &RedisCache{client: cli}
}

// noteKey 单条笔记缓存 key。
func noteKey(noteID string) string {
	return "note:" + noteID
}

// jitteredTTL 在基准 TTL 上叠加随机抖动
func jitteredTTL() time.Duration {
	return cacheTTLBase + time.Duration(rand.Int63n(int64(cacheTTLJitter)))
}

// GetNote 返回值约定：
//   - (doc, nil)             ：cache hit
//   - (nil, ErrCacheMiss)    ：cache miss，应回查 DB
//   - (nil, ErrCacheNegative)：命中空值缓存，直接返回 NotFound
//   - (nil, otherErr)        ：redis 自身故障；上层应降级为 DB 直查（不阻塞主流程）
func (c *RedisCache) GetNote(ctx context.Context, noteID string) (*noteDoc, error) {
	raw, err := c.client.Get(ctx, noteKey(noteID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	if raw == negCacheValue {
		return nil, ErrCacheNegative
	}

	var d noteDoc
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		// 反序列化失败说明 cache 数据被破坏，回退为 cache miss 走 DB。
		log.WarnContextf(ctx, "redis cache unmarshal fail, key=%s, err=%v", noteKey(noteID), err)
		return nil, ErrCacheMiss
	}
	return &d, nil
}

// SetNote 写入正向缓存（命中数据）。TTL 带抖动。
func (c *RedisCache) SetNote(ctx context.Context, n *noteDoc) error {
	data, err := json.Marshal(n) // 序列化
	if err != nil {
		return err
	}
	ttl := jitteredTTL()
	return c.client.Set(ctx, noteKey(n.NoteID), string(data), ttl).Err()
}

// SetNegative 写入空值缓存（防穿透）。短 TTL：避免写入新数据后短期内还查不到。
func (c *RedisCache) SetNegative(ctx context.Context, noteID string) error {
	return c.client.Set(ctx, noteKey(noteID), negCacheValue, negCacheTTL).Err()
}

// DelNote 删除正向缓存。
func (c *RedisCache) DelNote(ctx context.Context, noteID string) error {
	return c.client.Del(ctx, noteKey(noteID)).Err()
}
