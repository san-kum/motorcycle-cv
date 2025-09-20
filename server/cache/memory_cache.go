package cache

import (
	"context"
	"crypto/md5"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type MemoryCache struct {
	items   map[string]*CacheItem
	mutex   sync.RWMutex
	maxSize int
	ttl     time.Duration
	logger  *zap.Logger
	cleanup *time.Ticker
	stopCh  chan struct{}
}

type CacheItem struct {
	Value       interface{}
	ExpiresAt   time.Time
	LastUsed    time.Time
	AccessCount int64
}

func NewMemoryCache(maxSize int, ttl time.Duration, logger *zap.Logger) *MemoryCache {
	cache := &MemoryCache{
		items:   make(map[string]*CacheItem),
		maxSize: maxSize,
		ttl:     ttl,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}

	cache.cleanup = time.NewTicker(1 * time.Minute)
	go cache.cleanupExpired()

	return cache
}

func (c *MemoryCache) Set(ctx context.Context, key string, value interface{}) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.items) >= c.maxSize {
		c.evictLRU()
	}

	c.items[key] = &CacheItem{
		Value:       value,
		ExpiresAt:   time.Now().Add(c.ttl),
		LastUsed:    time.Now(),
		AccessCount: 1,
	}

	return nil
}

func (c *MemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
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

	c.mutex.Lock()
	item.LastUsed = time.Now()
	item.AccessCount++
	c.mutex.Unlock()

	if destPtr, ok := dest.(*interface{}); ok {
		*destPtr = item.Value
	}

	return nil
}

func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.items, key)
	return nil
}

func (c *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
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

func (c *MemoryCache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.items) >= c.maxSize {
		c.evictLRU()
	}

	c.items[key] = &CacheItem{
		Value:       value,
		ExpiresAt:   time.Now().Add(ttl),
		LastUsed:    time.Now(),
		AccessCount: 1,
	}

	return nil
}

func (c *MemoryCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
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

func (c *MemoryCache) Increment(ctx context.Context, key string) (int64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, exists := c.items[key]
	if !exists {
		c.items[key] = &CacheItem{
			Value:       int64(1),
			ExpiresAt:   time.Now().Add(c.ttl),
			LastUsed:    time.Now(),
			AccessCount: 1,
		}
		return 1, nil
	}

	if time.Now().After(item.ExpiresAt) {
		item.Value = int64(1)
		item.ExpiresAt = time.Now().Add(c.ttl)
		item.LastUsed = time.Now()
		item.AccessCount = 1
		return 1, nil
	}

	if count, ok := item.Value.(int64); ok {
		item.Value = count + 1
		item.LastUsed = time.Now()
		item.AccessCount++
		return item.Value.(int64), nil
	}

	item.Value = int64(1)
	item.LastUsed = time.Now()
	item.AccessCount++
	return 1, nil
}

func (c *MemoryCache) IncrementWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, exists := c.items[key]
	if !exists {
		c.items[key] = &CacheItem{
			Value:       int64(1),
			ExpiresAt:   time.Now().Add(ttl),
			LastUsed:    time.Now(),
			AccessCount: 1,
		}
		return 1, nil
	}

	if count, ok := item.Value.(int64); ok {
		item.Value = count + 1
	} else {
		item.Value = int64(1)
	}
	item.ExpiresAt = time.Now().Add(ttl)
	item.LastUsed = time.Now()
	item.AccessCount++

	return item.Value.(int64), nil
}
func (c *MemoryCache) GetStats(ctx context.Context) (*CacheStats, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	now := time.Now()
	expiredCount := 0
	totalAccessCount := int64(0)

	for _, item := range c.items {
		if now.After(item.ExpiresAt) {
			expiredCount++
		}
		totalAccessCount += item.AccessCount
	}

	stats := &CacheStats{
		Connected: true,
		Info: fmt.Sprintf("items=%d,expired=%d,access_count=%d,max_size=%d",
			len(c.items), expiredCount, totalAccessCount, c.maxSize),
	}

	return stats, nil
}

func (c *MemoryCache) Close() error {
	if c.cleanup != nil {
		c.cleanup.Stop()
	}
	close(c.stopCh)
	return nil
}

func (c *MemoryCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, item := range c.items {
		if oldestKey == "" || item.LastUsed.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.LastUsed
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

func (c *MemoryCache) cleanupExpired() {
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

func GenerateCacheKey(components ...string) string {
	h := md5.New()
	for _, component := range components {
		h.Write([]byte(component))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
