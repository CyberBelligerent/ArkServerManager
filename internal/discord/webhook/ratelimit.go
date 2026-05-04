package webhook

import (
	"context"
	"sync"
	"time"
)

type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	max        float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func newTokenBucket(max, refillRate float64) *tokenBucket {
	return &tokenBucket{tokens: max, max: max, refillRate: refillRate, lastRefill: time.Now()}
}

func (b *tokenBucket) Take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.max {
		b.tokens = b.max
	}
	b.lastRefill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (b *tokenBucket) Wait(ctx context.Context) error {
	for {
		if b.Take() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
