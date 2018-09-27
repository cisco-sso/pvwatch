package main

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	tests := []struct {
		key    string
		wait   time.Duration
		result bool
	}{
		{
			key:    "abc",
			wait:   1 * time.Second,
			result: false,
		},
		{
			key:    "abc",
			wait:   3 * time.Second,
			result: true,
		},
	}
	caches := make([]Cache, len(tests))
	maxTime := 0 * time.Second
	for i, tt := range tests {
		caches[i] = NewCache(tt.wait)
		caches[i].Put(tt.key)
		maxTime = max(maxTime, tt.wait)
	}
	time.Sleep(2 * time.Second)
	for i, tt := range tests {
		if tt.result != caches[i].Contains(tt.key) {
			t.Errorf("Test %d. for %v time failed", i, tt.wait)
		}
	}
	time.Sleep(maxTime)
	for i, tt := range tests {
		if caches[i].Contains(tt.key) {
			t.Errorf("Test %d. for %v should have expired", i, tt.wait)
		}
	}
}

func max(a, b time.Duration) time.Duration {
	if a < b {
		return b
	}
	return a
}
