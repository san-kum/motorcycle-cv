package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type RedisCache struct {
	items   map[string]*CacheItem
	mutex   sync.RWMutex
	ttl     time.Duration
	logger  *zap.Logger
	cleanup *time.Ticker
	stopCh  chan struct{}
}

func NewRedisCache(host string, port int, password string, db int, ttl time.Duration, logger *zap.Logger) (*RedisCache, error) {
	logger.Warn("Using memory cache instead of Redis",
		zap.String("host", host),
		zap.Int("port", port))

	cache := &RedisCache{
		items:  make(map[string]*CacheItem),
		ttl:    ttl,
		logger: logger,
		stopCh: make(chan struct{}),
	}

	cache.cleanup = time.NewTicker(1 * time.Minute)
	go cache.cleanupExpired()

	return cache, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.items[key] = &CacheItem{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}

	return nil
}

func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	c.mutex.RLock()
	item, exists := c.items[key]
	c.mutex.RUnlock()

	if !exists {
		return ErrCacheMiss
	}

	if time.Now().After(item.ExpiresAt) {
		c.mutex.Lock()
		delete(c.items, key)
		c.mutex.Unlock()
		return ErrCacheMiss
	}

	if destPtr, ok := dest.(*interface{}); ok {
		*destPtr = item.Value
	}

	return nil
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.items, key)
	return nil
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return false, nil
	}

	if time.Now().After(item.ExpiresAt) {
		return false, nil
	}

	return true, nil
}

func (c *RedisCache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.items[key] = &CacheItem{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}

	return nil
}

func (c *RedisCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return 0, ErrCacheMiss
	}

	if time.Now().After(item.ExpiresAt) {
		return 0, ErrCacheMiss
	}

	return time.Until(item.ExpiresAt), nil
}
func (c *RedisCache) Increment(ctx context.Context, key string) (int64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, exists := c.items[key]
	if !exists {
		c.items[key] = &CacheItem{
			Value:     int64(1),
			ExpiresAt: time.Now().Add(c.ttl),
		}
		return 1, nil
	}

	if time.Now().After(item.ExpiresAt) {
		item.Value = int64(1)
		item.ExpiresAt = time.Now().Add(c.ttl)
		return 1, nil
	}

	if count, ok := item.Value.(int64); ok {
		item.Value = count + 1
		return item.Value.(int64), nil
	}

	item.Value = int64(1)
	return 1, nil
}

func (c *RedisCache) IncrementWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, exists := c.items[key]
	if !exists {
		c.items[key] = &CacheItem{
			Value:     int64(1),
			ExpiresAt: time.Now().Add(ttl),
		}
		return 1, nil
	}

	if count, ok := item.Value.(int64); ok {
		item.Value = count + 1
	} else {
		item.Value = int64(1)
	}
	item.ExpiresAt = time.Now().Add(ttl)

	return item.Value.(int64), nil
}

func (c *RedisCache) GetStats(ctx context.Context) (*CacheStats, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	now := time.Now()
	expiredCount := 0

	for _, item := range c.items {
		if now.After(item.ExpiresAt) {
			expiredCount++
		}
	}

	stats := &CacheStats{
		Connected: true,
		Info: fmt.Sprintf("items=%d,expired=%d,ttl=%v",
			len(c.items), expiredCount, c.ttl),
	}

	return stats, nil
}

func (c *RedisCache) Close() error {
	if c.cleanup != nil {
		c.cleanup.Stop()
	}
	close(c.stopCh)
	return nil
}

func (c *RedisCache) cleanupExpired() {
	for {
		select {
		case <-c.cleanup.C:
			c.mutex.Lock()
			now := time.Now()
			for key, item := range c.items {
				if now.After(item.ExpiresAt) {
					delete(c.items, key)
				}
			}
			c.mutex.Unlock()
		case <-c.stopCh:
			return
		}
	}
}

var ErrCacheMiss = fmt.Errorf("cache miss")
