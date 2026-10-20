package netobserve

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestDialSummarySeparatesLocalAndRemoteChanges(t *testing.T) {
	var key [32]byte
	key[0] = 1
	for _, tc := range []struct {
		address                      string
		path                         dialPath
		family, route, local, remote string
	}{
		{"192.0.2.1:3", dialPath{actual: 4, suggested: 4}, "ipv4", "selected", "same", "same"},
		{"192.0.2.1:3", dialPath{actual: 5, suggested: 4}, "ipv4", "different", "changed", "same"},
		{"192.0.2.2:3", dialPath{actual: 4, suggested: 4}, "ipv4", "selected", "same", "changed"},
		{"[2001:db8::1]:3", dialPath{actual: 0, suggested: 4}, "ipv6", "suggested", "same", "changed"},
		{"192.0.2.1:3", dialPath{}, "ipv4", "unknown", "unknown", "same"},
	} {
		v := summarizeDial(DialAttempt{Address: tc.address, Local: "192.0.2.9:8", Phase: "tcp", Duration: time.Millisecond}, tc.path, key, true, "192.0.2.1:3", 4)
		if !v.Valid() || v.Family != tc.family || v.Route != tc.route || v.LocalChange != tc.local || v.EndpointChange != tc.remote || v.DNS != "not_applicable" || v.Micros != 1000 {
			t.Fatalf("%+v", v)
		}
		data, _ := json.Marshal(v)
		if strings.Contains(string(data), "192.0.2") || strings.Contains(string(data), "2001:db8") {
			t.Fatal("address leaked")
		}
	}
	a := DialAttempt{Address: "192.0.2.1:3", Phase: "tls_auth", Err: errors.New("private TLS data")}
	v := summarizeDial(a, dialPath{actual: 4, suggested: 4}, key, false, "", 0)
	if !v.Valid() || v.Interface != "" || v.SuggestedInterface != "" || v.Route != "unknown" || v.Result != "failed" {
		t.Fatal(v)
	}
	key[0] = 2
	x := summarizeDial(a, dialPath{actual: 4, suggested: 4}, key, true, "", 0)
	key[0] = 3
	y := summarizeDial(a, dialPath{actual: 4, suggested: 4}, key, true, "", 0)
	if x.Interface == y.Interface {
		t.Fatal("identifiers stable across monitors")
	}
}

func TestDialNativeQueryTimeoutDoesNotCreateAnotherWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer dialProbeBusy.Store(false)
		release := make(chan struct{})
		var calls atomic.Int32
		query := func(DialAttempt) dialPath { calls.Add(1); <-release; return dialPath{actual: 9, suggested: 9} }
		if p := boundedDialPath(context.Background(), DialAttempt{}, query); p != (dialPath{}) {
			t.Fatal(p)
		}
		for i := 0; i < 50; i++ {
			if p := boundedDialPath(context.Background(), DialAttempt{}, query); p != (dialPath{}) {
				t.Fatal(p)
			}
		}
		if calls.Load() != 1 {
			t.Fatal("parallel native query")
		}
		close(release)
		synctest.Wait()
		if p := boundedDialPath(context.Background(), DialAttempt{}, func(DialAttempt) dialPath { return dialPath{actual: 3, suggested: 3} }); p.actual != 3 {
			t.Fatal("late result reached replacement", p)
		}
	})
}

func TestDialObserverNeverWaitsForFullQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &DialMonitor{ctx: ctx, queue: make(chan DialAttempt, 1)}
	for i := 0; i < 1000; i++ {
		m.Observe(DialAttempt{})
	}
	if len(m.queue) != 1 {
		t.Fatal("queue unbounded")
	}
}
