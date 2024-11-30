package cache

import (
	"sync"
	"time"
)

type item[V any] struct {
	value  V
	expiry time.Time
}

func (i item[V]) isExpired() bool {
	return i.expiry.Before(time.Now())
}

type TTLCache[K comparable, V any] struct {
	items map[K]item[V]
	mu    sync.Mutex
}

func NewTTLCache[K comparable, V any]() *TTLCache[K, V] {
	c := &TTLCache[K, V]{
		items: make(map[K]item[V]),
	}

	go func() {
		for range time.Tick(5 * time.Second) {
			c.mu.Lock()
			for k, v := range c.items {
				if v.isExpired() {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		}
	}()

	return c
}

func (c *TTLCache[K, V]) Set(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = item[V]{
		value:  value,
		expiry: time.Now().Add(ttl),
	}
}

func (c *TTLCache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	currItem, ok := c.items[key]
	if !ok {
		return currItem.value, false
	}

	if currItem.isExpired() {
		delete(c.items, key)

		return currItem.value, false
	}

	return currItem.value, true
}

func (c *TTLCache[K, V]) Pop(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	currItem, ok := c.items[key]
	if !ok {
		return currItem.value, false
	}

	delete(c.items, key)

	if currItem.isExpired() {
		return currItem.value, false
	}

	return currItem.value, true
}

func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}
