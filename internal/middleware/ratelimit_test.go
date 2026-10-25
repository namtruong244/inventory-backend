package middleware

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	l := NewRateLimiter(2, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }

	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("first two attempts must pass")
	}
	if l.Allow("a") {
		t.Fatal("third attempt must be blocked")
	}
	if !l.Allow("b") {
		t.Fatal("other keys are independent")
	}
	now = now.Add(2 * time.Minute)
	if !l.Allow("a") {
		t.Fatal("window must reset")
	}
}
