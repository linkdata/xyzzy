package ui

import (
	"net/netip"
	"sync"
	"time"
)

const (
	createRoomMinuteBurst = 10
	createRoomHourBurst   = 50
)

// Keep client keys aligned with JaWS clientBucketKey in requestpool.go.
var nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

type createRoomLimiter struct {
	mu        sync.Mutex
	now       func() time.Time
	buckets   map[string]*createRoomBucket
	lastSweep time.Time
}

type createRoomBucket struct {
	minuteTokens float64
	hourTokens   float64
	minuteSeen   time.Time
	hourSeen     time.Time
}

func newCreateRoomLimiter() (result *createRoomLimiter) {
	result = &createRoomLimiter{
		now:     time.Now,
		buckets: make(map[string]*createRoomBucket),
	}
	return
}

// Allow reports whether one create-room attempt may proceed for ip.
func (l *createRoomLimiter) Allow(ip string) (ok bool) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) >= time.Minute {
		for key, bucket := range l.buckets {
			if now.Sub(bucket.hourSeen) >= time.Hour {
				delete(l.buckets, key)
			}
		}
		l.lastSweep = now
	}

	addr, err := netip.ParseAddr(ip)
	if err == nil {
		addr = addr.Unmap()
		switch {
		case nat64Prefix.Contains(addr):
			a := addr.As16()
			addr = netip.AddrFrom4([4]byte(a[12:]))
		case addr.Is6():
			addr = netip.PrefixFrom(addr, 64).Masked().Addr()
		}
		ip = addr.String()
	}

	bucket := l.buckets[ip]
	if bucket == nil {
		bucket = &createRoomBucket{
			minuteTokens: createRoomMinuteBurst,
			hourTokens:   createRoomHourBurst,
			minuteSeen:   now,
			hourSeen:     now,
		}
		l.buckets[ip] = bucket
	}

	bucket.minuteTokens = refillTokens(bucket.minuteTokens, bucket.minuteSeen, now, 5.0/60.0, createRoomMinuteBurst)
	bucket.hourTokens = refillTokens(bucket.hourTokens, bucket.hourSeen, now, 50.0/3600.0, createRoomHourBurst)
	bucket.minuteSeen = now
	bucket.hourSeen = now

	if bucket.minuteTokens >= 1 && bucket.hourTokens >= 1 {
		bucket.minuteTokens--
		bucket.hourTokens--
		ok = true
	}
	return
}

func refillTokens(tokens float64, lastSeen, now time.Time, rate float64, burst float64) (result float64) {
	result = tokens + now.Sub(lastSeen).Seconds()*rate
	if result > burst {
		result = burst
	}
	return
}
