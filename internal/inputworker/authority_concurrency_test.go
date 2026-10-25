package inputworker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAuthorityInputAndRecoveryDoNotWaitForClosingHandles(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	queries := 0
	a := &Authority{
		checkInput:         func() error { queries++; return nil },
		checkInputObserved: func(context.Context) error { queries++; return nil },
		checkRecovery:      func() error { queries++; return nil },
		closeHandles:       func() error { close(entered); <-release; return nil },
	}
	closing := make(chan error, 1)
	go func() { closing <- a.Close() }()
	<-entered // Close owns the write lock and is still retiring native handles.
	type result struct {
		name string
		err  error
	}
	done := make(chan result, 3)
	for name, check := range map[string]func() error{
		"input":    func() error { return a.check(false) },
		"observed": func() error { return a.checkObserved(context.Background(), false) },
		"recovery": func() error { return a.check(true) },
	} {
		go func() { done <- result{name, check()} }()
	}
	returned := 0
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
waiting:
	for returned < 3 {
		select {
		case r := <-done:
			returned++
			if r.err != ErrAuthority {
				t.Errorf("%s accepted a closing proof: %v", r.name, r.err)
			}
		case <-timer.C:
			t.Errorf("%d/3 checks waited for native handle retirement", 3-returned)
			break waiting
		}
	}
	close(release)
	if err := <-closing; err != nil {
		t.Fatal(err)
	}
	for ; returned < 3; returned++ {
		<-done
	} // Join fixtures even against the old code.
	if queries != 0 {
		t.Fatal("closing proof queried native handles", queries)
	}
}

func TestAuthorityConcurrentClosePreservesFirstFailure(t *testing.T) {
	closes := 0
	authority := &Authority{
		checkInput:    func() error { return nil },
		checkRecovery: func() error { return nil },
		closeHandles:  func() error { closes++; return errors.New("private native close failure") },
	}
	var callers sync.WaitGroup
	for range 8 {
		callers.Go(func() {
			if err := authority.Close(); err != ErrAuthority {
				t.Error("repeated close erased failure or leaked native details", err)
			}
		})
	}
	callers.Wait()
	if closes != 1 || authority.check(false) != ErrAuthority || authority.check(true) != ErrAuthority {
		t.Fatal("failed retirement retried handles or revived authority", closes)
	}
}

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
