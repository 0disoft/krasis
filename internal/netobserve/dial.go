package netobserve

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/netip"
	"strconv"
	"sync/atomic"
	"time"
)

// Raw addresses exist only in the bounded in-memory observation queue.
type DialAttempt struct {
	Address, Local, Phase string
	Duration              time.Duration
	Err                   error
}

type DialObservation struct {
	Phase              string `json:"phase"`
	Family             string `json:"family"`
	DNS                string `json:"dns"`
	Result             string `json:"result"`
	Micros             int64  `json:"micros"`
	Route              string `json:"route"`
	Interface          string `json:"interface,omitempty"`
	SuggestedInterface string `json:"suggestedInterface,omitempty"`
	LocalChange        string `json:"localChange"`
	EndpointChange     string `json:"endpointChange"`
	RouteRevision      uint64 `json:"routeRevision"`
}

func (v DialObservation) Valid() bool {
	in := func(s string, values ...string) bool {
		for _, v := range values {
			if s == v {
				return true
			}
		}
		return false
	}
	id := func(s string) bool {
		b, e := hex.DecodeString(s)
		return e == nil && len(b) == 8 && s == hex.EncodeToString(b)
	}
	return in(v.Phase, "tcp", "tls_auth") && in(v.Family, "ipv4", "ipv6", "unknown") && in(v.DNS, "not_applicable", "unknown") &&
		in(v.Result, "ok", "failed") && v.Micros >= 0 && in(v.Route, "selected", "different", "suggested", "unknown") &&
		(v.Interface == "" || id(v.Interface)) && (v.SuggestedInterface == "" || id(v.SuggestedInterface)) &&
		in(v.LocalChange, "unknown", "same", "changed") && in(v.EndpointChange, "unknown", "same", "changed") &&
		(v.Route != "selected" || v.Interface != "" && v.Interface == v.SuggestedInterface) &&
		(v.Route != "different" || v.Interface != "" && v.SuggestedInterface != "" && v.Interface != v.SuggestedInterface) &&
		(v.Route != "suggested" || v.Interface == "" && v.SuggestedInterface != "")
}

type dialPath struct {
	actual, suggested int
	route             [32]byte // Private route comparison, never exported.
}

var dialProbeBusy atomic.Bool

// A stuck native query owns at most one process-wide worker, never a socket.
func boundedDialPath(ctx context.Context, a DialAttempt, query func(DialAttempt) dialPath) dialPath {
	if ctx.Err() != nil || !dialProbeBusy.CompareAndSwap(false, true) {
		return dialPath{}
	}
	ch := make(chan dialPath, 1)
	go func() {
		var p dialPath
		defer func() { _ = recover(); dialProbeBusy.Store(false); ch <- p }()
		p = query(a)
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case p := <-ch:
		if ctx.Err() == nil {
			return p
		}
	case <-ctx.Done():
	case <-timer.C:
	}
	return dialPath{}
}

type DialMonitor struct {
	queue  chan DialAttempt
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

func StartDial(ctx context.Context, emit func(DialObservation)) *DialMonitor {
	ctx, cancel := context.WithCancel(ctx)
	m := &DialMonitor{queue: make(chan DialAttempt, 16), done: make(chan struct{}), ctx: ctx, cancel: cancel}
	go func() {
		defer close(m.done)
		var key [32]byte
		_, err := rand.Read(key[:])
		known := err == nil
		var previousEndpoint string
		var previousPath int
		var previousRoute [32]byte
		var revision uint64
		for {
			select {
			case <-ctx.Done():
				return
			case a := <-m.queue:
				p := boundedDialPath(ctx, a, sampleDialPath)
				v := summarizeDial(a, p, key, known, previousEndpoint, previousPath)
				if p.route != ([32]byte{}) {
					if previousRoute != p.route {
						revision++
						if previousRoute != ([32]byte{}) {
							v.LocalChange = "changed"
						}
					}
					v.RouteRevision = revision
					previousRoute = p.route
				}
				if a.Phase == "tcp" {
					previousEndpoint = a.Address
					if p.actual != 0 {
						previousPath = p.actual
					} else if p.suggested != 0 {
						previousPath = p.suggested
					}
				}
				if ctx.Err() == nil && v.Valid() && emit != nil {
					emit(v)
				}
			}
		}
	}()
	return m
}
func (m *DialMonitor) Observe(a DialAttempt) {
	if m.ctx.Err() != nil {
		return
	}
	select {
	case m.queue <- a:
	default:
	}
}
func (m *DialMonitor) Close() { m.cancel(); <-m.done }

func summarizeDial(a DialAttempt, p dialPath, key [32]byte, known bool, previousEndpoint string, previousPath int) DialObservation {
	v := DialObservation{Phase: a.Phase, Family: "unknown", DNS: "unknown", Result: "ok", Micros: max(0, a.Duration.Microseconds()), Route: "unknown", LocalChange: "unknown", EndpointChange: "unknown"}
	if a.Err != nil {
		v.Result = "failed"
	}
	if ip, err := netip.ParseAddrPort(a.Address); err == nil {
		v.DNS = "not_applicable"
		if ip.Addr().Is4() || ip.Addr().Is4In6() {
			v.Family = "ipv4"
		} else {
			v.Family = "ipv6"
		}
	}
	if previousEndpoint != "" {
		v.EndpointChange = "same"
		if previousEndpoint != a.Address {
			v.EndpointChange = "changed"
		}
	}
	path := p.actual
	if path == 0 {
		path = p.suggested
	}
	if path != 0 && previousPath != 0 {
		v.LocalChange = "same"
		if path != previousPath {
			v.LocalChange = "changed"
		}
	}
	if !known {
		return v
	}
	id := func(i int) string {
		if i == 0 {
			return ""
		}
		h := hmac.New(sha256.New, key[:])
		h.Write([]byte("interface:" + strconv.Itoa(i)))
		return hex.EncodeToString(h.Sum(nil)[:8])
	}
	v.Interface, v.SuggestedInterface = id(p.actual), id(p.suggested)
	if p.actual != 0 && p.suggested != 0 {
		v.Route = "selected"
		if p.actual != p.suggested {
			v.Route = "different"
		}
	} else if p.suggested != 0 {
		v.Route = "suggested"
	}
	return v
}

func sampleDialPath(a DialAttempt) dialPath {
	p := dialPath{}
	if remote, e := netip.ParseAddrPort(a.Address); e == nil {
		p.suggested, p.route = routeDetails(remote.Addr())
	}
	local, e := netip.ParseAddrPort(a.Local)
	if e != nil {
		return p
	}
	interfaces, e := net.Interfaces()
	if e != nil {
		return p
	}
	for _, i := range interfaces {
		addrs, e := i.Addrs()
		if e != nil {
			continue
		}
		for _, s := range addrs {
			prefix, e := netip.ParsePrefix(s.String())
			if e == nil && prefix.Addr().Unmap().WithZone("") == local.Addr().Unmap().WithZone("") {
				if p.actual != 0 && p.actual != i.Index {
					p.actual = 0
					return p
				}
				p.actual = i.Index
			}
		}
	}
	return p
}
