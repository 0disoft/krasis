package inputworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"
)

func TestBootstrapHandleTransferRequiresExactAuthenticationAndLiveAuthority(t *testing.T) {
	for _, frame := range [][]byte{nil, workerAuthenticatedFrame[:7], readyFrame[:], []byte{'K', 'R', 'A', '1', 1, 0, 0, 1}} {
		called := false
		_, err := bootstrapAfterAuthentication(context.Background(), bytes.NewReader(frame), func() error { return nil }, func() ([bootstrapSize]byte, error) {
			called = true
			return [bootstrapSize]byte{}, nil
		})
		if err != ErrAuthority || called {
			t.Fatal("handle transfer preceded authentication")
		}
	}
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		called := false
		_, err := bootstrapAfterAuthentication(ctx, bytes.NewReader(workerAuthenticatedFrame[:]), func() error {
			if canceled {
				cancel()
			} else {
				return ErrAuthority // SCM stop/pipe-owner or agent identity change.
			}
			return nil
		}, func() ([bootstrapSize]byte, error) {
			// Native transfer independently checks cancellation before duplication.
			if ctx.Err() != nil {
				return [bootstrapSize]byte{}, ErrAuthority
			}
			called = true
			return [bootstrapSize]byte{}, nil
		})
		cancel()
		if err != ErrAuthority || called {
			t.Fatal("retired authority transferred handles")
		}
	}
	called := 0
	_, err := bootstrapAfterAuthentication(context.Background(), bytes.NewReader(workerAuthenticatedFrame[:]), func() error { called++; return nil }, func() ([bootstrapSize]byte, error) {
		if called != 1 {
			t.Fatal("transfer skipped fresh authority check")
		}
		called++
		return [bootstrapSize]byte{}, ErrAuthority // Partial duplicate/launch failure.
	})
	if err != ErrAuthority || called != 2 {
		t.Fatal("transfer failure was hidden or retried")
	}
}

func TestWorkerBootstrapRejectsOldProtocolAndMalformedHandles(t *testing.T) {
	lease, err := encodeLease(testLease(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	agent := [sha256.Size]byte{1, 2, 3}
	packet, err := encodeWorkerBootstrap(4, 8, agent, lease)
	if err != nil {
		t.Fatal(err)
	}
	if pipe, image, err := decodeWorkerBootstrapHandles(packet, agent); err != nil || pipe != 4 || image != 8 || !bytes.Equal(packet[bootstrapLeaseOffset:], lease[:]) {
		t.Fatal("bootstrap lost handle/lease binding")
	}
	for _, mutate := range []func(*[bootstrapSize]byte){
		func(data *[bootstrapSize]byte) { copy(data[:4], "KRB1") },
		func(data *[bootstrapSize]byte) { data[4] = 1 },
		func(data *[bootstrapSize]byte) { data[5] = 1 },
		func(data *[bootstrapSize]byte) { data[15] = 0 },
		func(data *[bootstrapSize]byte) { data[23] = 0 },
		func(data *[bootstrapSize]byte) { data[23] = 4 },
		func(data *[bootstrapSize]byte) { data[24] ^= 1 },
	} {
		bad := packet
		mutate(&bad)
		if _, _, err := decodeWorkerBootstrapHandles(bad, agent); err != ErrProtocol {
			t.Fatal("malformed bootstrap accepted")
		}
	}
	for _, handles := range [][2]uint64{{0, 8}, {4, 0}, {4, 4}, {^uint64(0), 8}, {4, ^uint64(0)}} {
		if _, err := encodeWorkerBootstrap(handles[0], handles[1], agent, lease); err != ErrProtocol {
			t.Fatal("invalid native handle encoded")
		}
	}
	for _, expected := range [][sha256.Size]byte{{}, {4, 5, 6}} {
		if _, _, err := decodeWorkerBootstrapHandles(packet, expected); err != ErrProtocol {
			t.Fatal("service pin was attributed to unrelated metadata digest")
		}
	}
	if _, err := encodeWorkerBootstrap(4, 8, [sha256.Size]byte{}, lease); err != ErrProtocol {
		t.Fatal("unverified agent digest encoded")
	}
}
