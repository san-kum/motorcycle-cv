package cache

import (
	"context"
	"time"
)

type Cache interface {
	Set(ctx context.Context, key string, value interface{}) error

	Get(ctx context.Context, key string, dest interface{}) error

	Delete(ctx context.Context, key string) error

	Exists(ctx context.Context, key string) (bool, error)

	SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	GetTTL(ctx context.Context, key string) (time.Duration, error)

	Increment(ctx context.Context, key string) (int64, error)

	IncrementWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error)

	GetStats(ctx context.Context) (*CacheStats, error)

	Close() error
}

type CacheStats struct {
	Connected bool   `json:"connected"`
	Info      string `json:"info"`
}
