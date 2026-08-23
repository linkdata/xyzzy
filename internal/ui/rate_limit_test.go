package ui

import (
	"testing"
	"time"
)

func TestCreateRoomLimiterAllowsPerIPBurst(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	limiter := newCreateRoomLimiter()
	limiter.now = func() time.Time { return now }

	for attempt := range createRoomMinuteBurst {
		if !limiter.Allow("192.0.2.1") {
			t.Fatalf("attempt %d rejected within burst", attempt+1)
		}
	}
	if limiter.Allow("192.0.2.1") {
		t.Fatal("attempt after burst allowed")
	}
	if !limiter.Allow("192.0.2.2") {
		t.Fatal("first attempt from another IP rejected")
	}
}
