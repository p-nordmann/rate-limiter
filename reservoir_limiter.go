package limiters

import (
	"context"
	"sync"
	"time"
)

type token struct{}

// Struct implementing the Limiter interface.
type reservoirLimiter struct {
	maxTokens      int
	refillDuration time.Duration
	in             chan token
	out            chan token
	numConcurrent  int
	mutex          sync.Mutex
}

// Creates a new reservoir limiter.
func NewReservoirLimiter(maxTokens int, refillDuration time.Duration) Limiter {
	return &reservoirLimiter{
		maxTokens:      maxTokens,
		refillDuration: refillDuration,
		in:             make(chan token),
		out:            make(chan token),
	}
}

// Blocks until a token is available or the context is canceled.
func (l *reservoirLimiter) Limit(ctx context.Context) error {
	l.mutex.Lock()
	if l.numConcurrent == 0 {
		l.numConcurrent = 2
		go l.manageTokens()
	} else {
		l.numConcurrent++
	}
	l.mutex.Unlock()
	select {
	case <-l.out:
		l.mutex.Lock()
		l.numConcurrent--
		l.mutex.Unlock()
		return nil
	case <-ctx.Done():
		l.mutex.Lock()
		l.numConcurrent--
		l.mutex.Unlock()
		return ctx.Err()
	}
}

// Manages the tokens in the reservoir (distribution and refill).
func (l *reservoirLimiter) manageTokens() {
	tokenCount := l.maxTokens
	for {
		switch tokenCount {
		case 0:
			<-l.in
			tokenCount++
		case l.maxTokens:
			l.mutex.Lock()
			if l.numConcurrent == 1 {
				// No one is waiting: free resources.
				l.numConcurrent = 0
				l.mutex.Unlock()
				return
			}
			l.mutex.Unlock()
			l.out <- token{}
			tokenCount--
			go l.refillTokens()
		default:
			select {
			case <-l.in:
				tokenCount++
			case l.out <- token{}:
				tokenCount--
			}
		}
	}
}

// Starts a ticker to refill missing tokens.
func (l *reservoirLimiter) refillTokens() {
	ticker := time.NewTicker(l.refillDuration)
	for range ticker.C {
		select {
		case l.in <- token{}:
			// Token refilled, wait for next tick.
		default:
			// Reservoir full, stop ticking.
			ticker.Stop()
			return
		}
	}
}
