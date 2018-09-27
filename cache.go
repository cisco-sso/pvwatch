package main

import (
	"sync"
	"time"
)

type Cache interface {
	Put(key string)
	Contains(Key string) bool
}

type c struct {
	c map[string]time.Time
	m *sync.Mutex
	t time.Duration
}

func NewCache(t time.Duration) *c {
	c := c{
		make(map[string]time.Time),
		&sync.Mutex{},
		t,
	}
	go c.gc()
	return &c
}

func (c *c) Put(key string) {
	c.m.Lock()
	defer c.m.Unlock()
	c.c[key] = time.Now()
}

func (c *c) get(key string) (k time.Time, ok bool) {
	c.m.Lock()
	defer c.m.Unlock()
	k, ok = c.c[key]
	return
}

func (c *c) Contains(key string) bool {
	now := time.Now()
	t, ok := c.get(key)
	return ok && now.Add(-c.t).Before(t)
}

func (c *c) delete(k string) {
	c.m.Lock()
	defer c.m.Unlock()
	delete(c.c, k)
}

func (c *c) gc() {
	for {
		time.Sleep(c.t)
		now := time.Now()
		for k, v := range c.c {
			if now.After(v) {
				c.delete(k)
			}
		}
	}
}
