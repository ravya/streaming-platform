package resilience

import (
	"errors"
	"testing"
	"time"
)

func TestBreakerLifecycle(t *testing.T) {
	now := time.Now()
	b := NewBreaker(2, time.Minute)
	b.now = func() time.Time { return now }
	boom := errors.New("boom")

	_ = b.Do(func() error { return boom })
	_ = b.Do(func() error { return boom })
	if b.State() != Open {
		t.Fatalf("want open, got %s", b.State())
	}
	if err := b.Do(func() error { t.Fatal("must not call fn when open"); return nil }); !errors.Is(err, ErrOpen) {
		t.Fatalf("want ErrOpen, got %v", err)
	}
	now = now.Add(2 * time.Minute) // cooldown elapsed -> half-open trial
	if err := b.Do(func() error { return nil }); err != nil || b.State() != Closed {
		t.Fatalf("trial success should close breaker; err=%v state=%s", err, b.State())
	}
}
