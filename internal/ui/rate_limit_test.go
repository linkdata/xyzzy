package ui

import (
	"net/http"
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

func TestCreateRoomLimiterEnforcesHourlyRate(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	limiter := newCreateRoomLimiter()
	limiter.now = func() time.Time { return now }

	for attempt := 1; attempt <= 60; attempt++ {
		allowed := limiter.Allow("192.0.2.1")
		if attempt < 60 && !allowed {
			t.Fatalf("attempt %d rejected before hourly limit", attempt)
		}
		if attempt == 60 && allowed {
			t.Fatal("attempt 60 allowed beyond hourly rate")
		}
		now = now.Add(12 * time.Second)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		request    *http.Request
		remoteAddr string
		want       string
	}{
		{name: "nil request", want: "unknown"},
		{name: "empty address", request: new(http.Request), want: "unknown"},
		{name: "IPv4 with port", request: new(http.Request), remoteAddr: "192.0.2.1:443", want: "192.0.2.1"},
		{name: "IPv6 with port", request: new(http.Request), remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "unsplit address", request: new(http.Request), remoteAddr: " host.example ", want: "host.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request != nil {
				tt.request.RemoteAddr = tt.remoteAddr
			}
			if got := clientIP(tt.request); got != tt.want {
				t.Fatalf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
