package inputworker

import (
	"sync"
	"testing"
	"time"
)

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
