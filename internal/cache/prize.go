package cache

import (
	"context"
	"sync"
	"time"

	"team-invite/internal/models"
)

type PrizeConfigLoader func(context.Context) ([]models.PrizeConfigItem, error)

type PrizeConfigCache struct {
	mu       sync.RWMutex
	value    []models.PrizeConfigItem
	expires  time.Time
	ttl      time.Duration
	loadFunc PrizeConfigLoader
}

func NewPrizeConfigCache(ttl time.Duration, loader PrizeConfigLoader) *PrizeConfigCache {
	return &PrizeConfigCache{
		ttl:      ttl,
		loadFunc: loader,
	}
}

func (c *PrizeConfigCache) Get(ctx context.Context) ([]models.PrizeConfigItem, error) {
	c.mu.RLock()
	if time.Now().Before(c.expires) && c.value != nil {
		defer c.mu.RUnlock()
		return c.value, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.expires) && c.value != nil {
		return c.value, nil
	}
	items, err := c.loadFunc(ctx)
	if err != nil {
		return nil, err
	}
	c.value = items
	c.expires = time.Now().Add(c.ttl)
	return items, nil
}

func (c *PrizeConfigCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = nil
	c.expires = time.Time{}
}
