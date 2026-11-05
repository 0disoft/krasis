//go:build windows

package peercredentials

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUserSIDCacheConcurrentGetLoadsOnce(t *testing.T) {
	const (
		callers = 32
		wantSID = "S-1-5-21-1000"
	)
	var cache userSIDCache
	var loads atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	load := func() (string, error) {
		if loads.Add(1) == 1 {
			close(started)
		}
		<-release
		return wantSID, nil
	}

	type result struct {
		sid string
		err error
	}
	begin := make(chan struct{})
	results := make(chan result, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for range callers {
		go func() {
			defer workers.Done()
			<-begin
			sid, err := cache.get(load)
			results <- result{sid: sid, err: err}
		}()
	}
	close(begin)
	<-started
	close(release)
	workers.Wait()
	close(results)

	for got := range results {
		if got.err != nil || got.sid != wantSID {
			t.Fatalf("concurrent SID = %q, %v", got.sid, got.err)
		}
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("native SID loaded %d times, want 1", got)
	}
}

func TestUserSIDCacheRetriesFailureAndReusesSuccess(t *testing.T) {
	cache := userSIDCache{}
	loads := 0
	load := func() (string, error) {
		loads++
		switch loads {
		case 1:
			return "", errors.New("transient token failure")
		case 2:
			return "", nil
		default:
			return "S-1-5-21-1000", nil
		}
	}

	if sid, err := cache.get(load); err == nil || sid != "" {
		t.Fatalf("token failure was cached: %q, %v", sid, err)
	}
	if sid, err := cache.get(load); err == nil || sid != "" {
		t.Fatalf("empty identity was cached: %q, %v", sid, err)
	}
	if sid, err := cache.get(load); err != nil || sid != "S-1-5-21-1000" {
		t.Fatalf("valid identity was not loaded: %q, %v", sid, err)
	}
	for range 10 {
		if sid, err := cache.get(load); err != nil || sid != "S-1-5-21-1000" {
			t.Fatalf("cached identity changed: %q, %v", sid, err)
		}
	}
	if loads != 3 {
		t.Fatalf("process identity loaded %d times, want 3", loads)
	}
}

func TestCurrentUserSIDStringMatchesNativeLoad(t *testing.T) {
	cached, err := currentUserSIDString()
	if err != nil {
		t.Fatal(err)
	}
	native, err := loadCurrentUserSIDString()
	if err != nil {
		t.Fatal(err)
	}
	if cached == "" || cached != native {
		t.Fatalf("cached SID = %q, native SID = %q", cached, native)
	}
	if again, err := currentUserSIDString(); err != nil || again != native {
		t.Fatalf("cached SID changed: %q, %v; native %q", again, err, native)
	}
}

func BenchmarkCurrentUserSIDString(b *testing.B) {
	if _, err := currentUserSIDString(); err != nil {
		b.Fatal(err)
	}
	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if sid, err := currentUserSIDString(); err != nil || sid == "" {
				b.Fatalf("cached SID = %q, %v", sid, err)
			}
		}
	})
	b.Run("uncached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if sid, err := loadCurrentUserSIDString(); err != nil || sid == "" {
				b.Fatalf("native SID = %q, %v", sid, err)
			}
		}
	})
}
