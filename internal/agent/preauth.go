package agent

import (
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultPerAddressPreAuthBurst          = 8
	DefaultMaxTrackedPreAuthAddresses      = 256
	DefaultPerAddressPreAuthRefillInterval = time.Second
	DefaultPreAuthAddressIdleTTL           = 5 * time.Minute
)

type PreAuthPolicy struct {
	HandshakeTimeout         time.Duration
	MaxConcurrent            int
	PerAddressBurst          int
	PerAddressRefillInterval time.Duration
	MaxTrackedAddresses      int
	AddressIdleTTL           time.Duration
}

func DefaultPreAuthPolicy() PreAuthPolicy {
	return PreAuthPolicy{
		HandshakeTimeout:         DefaultPreAuthHandshakeTimeout,
		MaxConcurrent:            MaxConcurrentPreAuthHandshakes,
		PerAddressBurst:          DefaultPerAddressPreAuthBurst,
		PerAddressRefillInterval: DefaultPerAddressPreAuthRefillInterval,
		MaxTrackedAddresses:      DefaultMaxTrackedPreAuthAddresses,
		AddressIdleTTL:           DefaultPreAuthAddressIdleTTL,
	}
}

func (policy PreAuthPolicy) validate() error {
	if policy.HandshakeTimeout <= 0 ||
		policy.MaxConcurrent <= 0 ||
		policy.PerAddressBurst <= 0 ||
		policy.PerAddressRefillInterval <= 0 ||
		policy.MaxTrackedAddresses <= 0 ||
		policy.AddressIdleTTL <= 0 {
		return errors.New("pre-auth policy values must be positive")
	}
	return nil
}

type PreAuthMetrics struct {
	Accepted               uint64
	RejectedNoPendingRound uint64
	RejectedRateLimited    uint64
	RejectedCapacity       uint64
	HandshakeTimeout       uint64
	AuthenticationFailure  uint64
	Canceled               uint64
}

type preAuthCounters struct {
	accepted               atomic.Uint64
	rejectedNoPendingRound atomic.Uint64
	rejectedRateLimited    atomic.Uint64
	rejectedCapacity       atomic.Uint64
	handshakeTimeout       atomic.Uint64
	authenticationFailure  atomic.Uint64
	canceled               atomic.Uint64
}

func (counters *preAuthCounters) snapshot() PreAuthMetrics {
	if counters == nil {
		return PreAuthMetrics{}
	}
	return PreAuthMetrics{
		Accepted:               counters.accepted.Load(),
		RejectedNoPendingRound: counters.rejectedNoPendingRound.Load(),
		RejectedRateLimited:    counters.rejectedRateLimited.Load(),
		RejectedCapacity:       counters.rejectedCapacity.Load(),
		HandshakeTimeout:       counters.handshakeTimeout.Load(),
		AuthenticationFailure:  counters.authenticationFailure.Load(),
		Canceled:               counters.canceled.Load(),
	}
}

type addressBucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

type addressRateLimiter struct {
	mu      sync.Mutex
	policy  PreAuthPolicy
	buckets map[string]*addressBucket
}

func newAddressRateLimiter(policy PreAuthPolicy) *addressRateLimiter {
	return &addressRateLimiter{
		policy:  policy,
		buckets: make(map[string]*addressBucket, policy.MaxTrackedAddresses),
	}
}

func (limiter *addressRateLimiter) allow(address net.Addr, now time.Time) bool {
	if limiter == nil {
		return false
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.removeIdle(now)
	key := preAuthAddressKey(address)
	bucket := limiter.buckets[key]
	if bucket == nil {
		if len(limiter.buckets) >= limiter.policy.MaxTrackedAddresses {
			limiter.evictOldest()
		}
		limiter.buckets[key] = &addressBucket{
			tokens:     float64(limiter.policy.PerAddressBurst - 1),
			lastRefill: now,
			lastSeen:   now,
		}
		return true
	}

	if elapsed := now.Sub(bucket.lastRefill); elapsed > 0 {
		bucket.tokens += float64(elapsed) / float64(limiter.policy.PerAddressRefillInterval)
		if maximum := float64(limiter.policy.PerAddressBurst); bucket.tokens > maximum {
			bucket.tokens = maximum
		}
		bucket.lastRefill = now
	}
	bucket.lastSeen = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (limiter *addressRateLimiter) removeIdle(now time.Time) {
	for key, bucket := range limiter.buckets {
		if elapsed := now.Sub(bucket.lastSeen); elapsed >= limiter.policy.AddressIdleTTL {
			delete(limiter.buckets, key)
		}
	}
}

func (limiter *addressRateLimiter) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	for key, bucket := range limiter.buckets {
		if oldestKey == "" || bucket.lastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = bucket.lastSeen
		}
	}
	if oldestKey != "" {
		delete(limiter.buckets, oldestKey)
	}
}

func preAuthAddressKey(address net.Addr) string {
	if address == nil {
		return "<unknown>"
	}
	if tcpAddress, ok := address.(*net.TCPAddr); ok {
		if len(tcpAddress.IP) == 0 {
			return "<unknown>"
		}
		return tcpAddress.IP.String()
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return "<unknown>"
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return "<unknown>"
	}
	return host
}
