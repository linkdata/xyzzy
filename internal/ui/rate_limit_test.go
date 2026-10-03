package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linkdata/jaws"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
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

func TestCreateRoomLimiterGroupsIPv6Clients(t *testing.T) {
	limiter := newCreateRoomLimiter()
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	for i := range createRoomMinuteBurst {
		if !limiter.Allow(fmt.Sprintf("2001:db8:1:2::%x", i)) {
			t.Fatalf("address %d rejected within shared burst", i)
		}
	}
	if limiter.Allow("2001:db8:1:2::ffff") {
		t.Fatal("eleventh address in /64 allowed")
	}
	if !limiter.Allow("2001:db8:1:3::1") {
		t.Fatal("neighbouring /64 rejected")
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2", len(limiter.buckets))
	}
}

func TestCreateRoomLimiterSharesNAT64WithIPv4(t *testing.T) {
	limiter := newCreateRoomLimiter()
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	for range createRoomMinuteBurst {
		if !limiter.Allow("192.0.2.1") {
			t.Fatal("IPv4 rejected within burst")
		}
	}
	if limiter.Allow("64:ff9b::c000:201") || limiter.Allow("::ffff:192.0.2.1") {
		t.Fatal("NAT64 or mapped IPv4 escaped IPv4 limit")
	}
	if !limiter.Allow("64:ff9b::c000:202") {
		t.Fatal("different NAT64 IPv4 address rejected")
	}
}

func TestCreateRoomLimiterPrunesIdleBuckets(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	limiter := newCreateRoomLimiter()
	limiter.now = func() time.Time { return now }
	limiter.Allow("192.0.2.1")
	limiter.Allow("192.0.2.2")
	now = now.Add(59 * time.Minute)
	limiter.Allow("192.0.2.2")
	now = now.Add(time.Minute)
	limiter.Allow("192.0.2.3")
	if len(limiter.buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2 active buckets", len(limiter.buckets))
	}
}

func TestCreateRoomLimiterEnforcesHourlyRate(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	limiter := newCreateRoomLimiter()
	limiter.now = func() time.Time { return now }

	// Twelve seconds restores one minute token but only 1/6 of an hour
	// token, so attempt 60 is the first rejected by the hour bucket.
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

func TestCreateRoomUsesSessionAddress(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(fmt.Sprint("trusted=", trusted), func(t *testing.T) {
			app, _ := testApp(t)
			app.Jaws.TrustForwardedHeaders = trusted
			limitedIP := "127.0.0.1"
			if trusted {
				limitedIP = "192.0.2.1"
			}
			for range createRoomMinuteBurst {
				if !app.createRoomLimiter.Allow(limitedIP) {
					t.Fatal("could not fill limiter bucket")
				}
			}
			for _, client := range []string{"192.0.2.1", "192.0.2.1", "192.0.2.2"} {
				req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
				req.RemoteAddr = "127.0.0.1:12345"
				req.Header.Set("X-Forwarded-For", client)
				var player *game.Player
				app.Jaws.SessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					player = app.player(app.Jaws.GetSession(r), r)
					rq := app.Jaws.NewRequest(w, r)
					elem := rq.NewElement(jui.NewButton("Create"))
					dot := templateDot{App: app, Player: player}
					if err := dot.CreateRoomButton().JawsClick(elem, jaws.Click{}); err != nil {
						t.Fatal(err)
					}
				})).ServeHTTP(httptest.NewRecorder(), req)
				wantRoom := trusted && client == "192.0.2.2"
				if got := player.Room() != nil; got != wantRoom {
					t.Fatalf("client %s created room = %t, want %t", client, got, wantRoom)
				}
			}
		})
	}
}
