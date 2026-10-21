package inputworker

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSourceAuthorityDoesNotWaitForClosingProof(t *testing.T) {
	queried := false
	a := &Authority{checkInput: func() error { queried = true; return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	a.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- a.checkSource() }()
	select {
	case err := <-done:
		a.mu.Unlock()
		if !errors.Is(err, ErrAuthority) || queried {
			t.Fatal("closing authority queried handles or accepted capture", err)
		}
	case <-time.After(time.Second):
		a.mu.Unlock()
		t.Fatal("source callback waited for proof retirement")
	}
	if err := a.checkSource(); err != nil || !queried {
		t.Fatal("available proof skipped fresh authority check", err)
	}
	if err := a.Close(); err != nil || a.checkSource() != ErrAuthority {
		t.Fatal("closed proof accepted input", err)
	}
}

func TestAuthorityChecksOverlapButCloseJoinsBothReaders(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	closed := make(chan struct{})
	a := &Authority{checkInput: func() error { entered <- struct{}{}; <-release; return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { close(closed); return nil }}
	done := make(chan error, 2)
	for range 2 {
		go func() { done <- a.check(false) }()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("independent read-only authentication serialized")
		}
	}
	closing := make(chan error, 1)
	go func() { closing <- a.Close() }()
	select {
	case <-closed:
		t.Fatal("proof handles closed during a live check")
	default:
	}
	once.Do(func() { close(release) })
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if err := <-closing; err != nil {
		t.Fatal(err)
	}
	if a.check(false) == nil {
		t.Fatal("closed authority accepted a fresh check")
	}
}

func TestPresenceChecksDoNotWaitForProofRetirement(t *testing.T) {
	for _, source := range []bool{false, true} {
		queried := false
		a := &Authority{checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, checkPresent: func() error { queried = true; return nil }, closeHandles: func() error { return nil }}
		check := a.checkHeartbeat
		if source {
			check = a.checkSourceHeartbeat
		}
		a.mu.Lock()
		done := make(chan error, 1)
		go func() { done <- check() }()
		select {
		case err := <-done:
			a.mu.Unlock()
			if !errors.Is(err, ErrAuthority) || queried {
				t.Fatal("closing proof queried handles or accepted presence", err)
			}
		case <-time.After(time.Second):
			a.mu.Unlock()
			<-done
			t.Fatal("presence check waited for proof retirement", source)
		}
		if err := check(); err != nil || !queried {
			t.Fatal("available presence proof was not checked freshly", err)
		}
		if err := a.Close(); err != nil || check() != ErrAuthority {
			t.Fatal("closed proof accepted presence", err)
		}
	}
}
