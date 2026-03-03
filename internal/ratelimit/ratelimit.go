package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	limiters map[string]*entry
	mu       sync.Mutex
	rate     rate.Limit
	burst    int
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewLimiter(r rate.Limit, burst int) *Limiter {
	l := &Limiter{
		limiters: make(map[string]*entry),
		rate:     r,
		burst:    burst,
	}
	// clean up old limiters every minute
	go l.cleanup()
	return l
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.limiters[key]; !ok {
		l.limiters[key] = &entry{
			limiter: rate.NewLimiter(l.rate, l.burst),
		}
	}
	l.limiters[key].lastSeen = time.Now()
	return l.limiters[key].limiter.Allow()
}

func (l *Limiter) cleanup() {
	for {
		time.Sleep(time.Minute)
		l.mu.Lock()
		for key, e := range l.limiters {
			if time.Since(e.lastSeen) > 5*time.Minute {
				delete(l.limiters, key)
			}
		}
		l.mu.Unlock()
	}
}
