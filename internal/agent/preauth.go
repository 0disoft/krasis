package agent

import (
	"errors"
	"math"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

type ListenNetworkPolicy string

const (
	ListenNetworkTrustedPrivate ListenNetworkPolicy = "trusted-private"
)

func (policy ListenNetworkPolicy) validate() error {
	if policy != ListenNetworkTrustedPrivate {
		return errors.New("listener network policy must explicitly be trusted-private")
	}
	return nil
}

type PreAuthAdmissionConfig struct {
	GlobalTokensPerSecond    float64
	GlobalBurst              int
	PerSourceTokensPerSecond float64
	PerSourceBurst           int
	MaxTrackedSources        int
	SourceIdleTTL            time.Duration
}

func DefaultPreAuthAdmissionConfig() PreAuthAdmissionConfig {
	return PreAuthAdmissionConfig{
		GlobalTokensPerSecond:    16,
		GlobalBurst:              32,
		PerSourceTokensPerSecond: 4,
		PerSourceBurst:           8,
		MaxTrackedSources:        256,
		SourceIdleTTL:            2 * time.Minute,
	}
}

func (config PreAuthAdmissionConfig) validate() error {
	if config.GlobalTokensPerSecond <= 0 ||
		config.PerSourceTokensPerSecond <= 0 ||
		config.GlobalBurst <= 0 ||
		config.PerSourceBurst <= 0 ||
		config.MaxTrackedSources <= 0 ||
		config.SourceIdleTTL <= 0 {
		return errors.New("pre-auth admission limits must be positive")
	}
	if math.IsNaN(config.GlobalTokensPerSecond) ||
		math.IsInf(config.GlobalTokensPerSecond, 0) ||
		math.IsNaN(config.PerSourceTokensPerSecond) ||
		math.IsInf(config.PerSourceTokensPerSecond, 0) {
		return errors.New("pre-auth admission rates must be finite")
	}
	return nil
}

type preAuthDecision uint8

const (
	preAuthAdmitted preAuthDecision = iota
	preAuthRejectedInvalidSource
	preAuthRejectedSourceCapacity
	preAuthRejectedSourceRate
	preAuthRejectedGlobalRate
)

type preAuthAdmission struct {
	mu      sync.Mutex
	config  PreAuthAdmissionConfig
	global  tokenBucket
	sources map[netip.Addr]*sourceBucket
}

type sourceBucket struct {
	bucket   tokenBucket
	lastSeen time.Time
}

type tokenBucket struct {
	tokens  float64
	updated time.Time
}

func newPreAuthAdmission(config PreAuthAdmissionConfig) (*preAuthAdmission, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &preAuthAdmission{
		config:  config,
		sources: make(map[netip.Addr]*sourceBucket, config.MaxTrackedSources),
	}, nil
}

func (admission *preAuthAdmission) allow(remote net.Addr, now time.Time) preAuthDecision {
	if admission == nil || now.IsZero() {
		return preAuthRejectedInvalidSource
	}
	source, ok := remoteIP(remote)
	if !ok {
		return preAuthRejectedInvalidSource
	}

	admission.mu.Lock()
	defer admission.mu.Unlock()

	entry := admission.sources[source]
	if entry == nil {
		admission.pruneIdle(now)
		if len(admission.sources) >= admission.config.MaxTrackedSources {
			return preAuthRejectedSourceCapacity
		}
		entry = &sourceBucket{}
		admission.sources[source] = entry
	}
	entry.lastSeen = now
	if !entry.bucket.take(now, admission.config.PerSourceTokensPerSecond, admission.config.PerSourceBurst) {
		return preAuthRejectedSourceRate
	}
	if !admission.global.take(now, admission.config.GlobalTokensPerSecond, admission.config.GlobalBurst) {
		return preAuthRejectedGlobalRate
	}
	return preAuthAdmitted
}

func (admission *preAuthAdmission) pruneIdle(now time.Time) {
	for source, entry := range admission.sources {
		if entry == nil || now.Sub(entry.lastSeen) >= admission.config.SourceIdleTTL {
			delete(admission.sources, source)
		}
	}
}

func (bucket *tokenBucket) take(now time.Time, rate float64, burst int) bool {
	if bucket.updated.IsZero() {
		bucket.tokens = float64(burst)
		bucket.updated = now
	}
	if now.Before(bucket.updated) {
		return false
	}
	if elapsed := now.Sub(bucket.updated); elapsed > 0 {
		bucket.tokens = math.Min(float64(burst), bucket.tokens+elapsed.Seconds()*rate)
		bucket.updated = now
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func remoteIP(remote net.Addr) (netip.Addr, bool) {
	if remote == nil {
		return netip.Addr{}, false
	}
	if address, ok := remote.(*net.TCPAddr); ok {
		ip, valid := netip.AddrFromSlice(address.IP)
		if !valid {
			return netip.Addr{}, false
		}
		return ip.Unmap(), true
	}
	host, _, err := net.SplitHostPort(remote.String())
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

type PreAuthMetrics struct {
	Admitted               uint64
	RejectedBusy           uint64
	RejectedInvalidSource  uint64
	RejectedSourceCapacity uint64
	RejectedSourceRate     uint64
	RejectedGlobalRate     uint64
	HandshakeFailed        uint64
	Authenticated          uint64
}

type preAuthMetricCounters struct {
	admitted               atomic.Uint64
	rejectedBusy           atomic.Uint64
	rejectedInvalidSource  atomic.Uint64
	rejectedSourceCapacity atomic.Uint64
	rejectedSourceRate     atomic.Uint64
	rejectedGlobalRate     atomic.Uint64
	handshakeFailed        atomic.Uint64
	authenticated          atomic.Uint64
}

func (metrics *preAuthMetricCounters) recordDecision(decision preAuthDecision) {
	switch decision {
	case preAuthAdmitted:
		metrics.admitted.Add(1)
	case preAuthRejectedInvalidSource:
		metrics.rejectedInvalidSource.Add(1)
	case preAuthRejectedSourceCapacity:
		metrics.rejectedSourceCapacity.Add(1)
	case preAuthRejectedSourceRate:
		metrics.rejectedSourceRate.Add(1)
	case preAuthRejectedGlobalRate:
		metrics.rejectedGlobalRate.Add(1)
	}
}

func (metrics *preAuthMetricCounters) snapshot() PreAuthMetrics {
	if metrics == nil {
		return PreAuthMetrics{}
	}
	return PreAuthMetrics{
		Admitted:               metrics.admitted.Load(),
		RejectedBusy:           metrics.rejectedBusy.Load(),
		RejectedInvalidSource:  metrics.rejectedInvalidSource.Load(),
		RejectedSourceCapacity: metrics.rejectedSourceCapacity.Load(),
		RejectedSourceRate:     metrics.rejectedSourceRate.Load(),
		RejectedGlobalRate:     metrics.rejectedGlobalRate.Load(),
		HandshakeFailed:        metrics.handshakeFailed.Load(),
		Authenticated:          metrics.authenticated.Load(),
	}
}
