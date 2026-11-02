package cache

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoalescesConcurrentMisses(t *testing.T) {
	c := New[int](10)
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = c.Do("k", time.Minute, func() (int, error) { calls.Add(1); time.Sleep(50 * time.Millisecond); return 7, nil })
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("backend called %d times, want 1", calls.Load())
	}
	if _, hit, _ := c.Do("k", time.Minute, func() (int, error) { return 0, nil }); !hit {
		t.Fatal("expected cache hit")
	}
}
