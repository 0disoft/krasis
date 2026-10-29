package inputworker

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestIsolatedDeliveryRetainsNativeAuthorityWithoutDuplicateClientRPC(t *testing.T) {
	for _, isolated := range []bool{false, true} {
		for _, denied := range []bool{false, true} {
			t.Run(map[bool]string{false: "generic", true: "isolated"}[isolated]+map[bool]string{false: "/allowed", true: "/revoked"}[denied], func(t *testing.T) {
				server, conn := net.Pipe()
				defer server.Close()
				channel := clientChannel(conn, authorityService)
				var clientRPC, nativeRPC, desktopChecks atomic.Int32
				channel.authority.checkInput = func() error { clientRPC.Add(1); return nil }
				channel.authority.checkPresent = func() error { return nil }
				client, err := NewClient(context.Background(), channel, testLease())
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				if clientRPC.Load() != 1 || client.isolatedDelivery {
					t.Fatal("client admission did not obtain full fresh authority", clientRPC.Load())
				}
				client.isolatedDelivery = isolated
				clientRPC.Store(0)
				execute := nativeInputExecutor(func(ctx context.Context, work func() error) error {
					desktopChecks.Add(1)
					return work()
				}, func() error {
					nativeRPC.Add(1)
					if denied {
						return ErrAuthority
					}
					return nil
				}, func() bool { return true }, time.Now().Add(time.Minute), false)
				handler := PortableInputHandler(&recordingInput{}, execute)
				done := make(chan error, 1)
				go func() { done <- handler(server, server) }()
				err = client.Move(context.Background(), 1, 0)
				if wireErr := <-done; wireErr != nil {
					t.Fatal(wireErr)
				}
				wantClientRPC := int32(1)
				if isolated {
					wantClientRPC = 0
				}
				if clientRPC.Load() != wantClientRPC || nativeRPC.Load() != 1 || (err != nil) != denied || desktopChecks.Load() != map[bool]int32{false: 1, true: 0}[denied] {
					t.Fatal("delivery skipped native authority or repeated client RPC", err, clientRPC.Load(), nativeRPC.Load(), desktopChecks.Load())
				}
				if denied {
					if certainty, known := application.InjectionCertaintyOf(err); !known || certainty != application.InjectionNotApplied {
						t.Fatal("revoked native service accepted input or lost certainty", err)
					}
					if err := client.Move(context.Background(), 1, 0); err == nil || nativeRPC.Load() != 1 {
						t.Fatal("rejected input was retried", err)
					}
				}
			})
		}
	}
}

func TestIsolatedDeliveryRejectsDeadPinnedOwnerBeforeWriting(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &admissionObservedConn{Conn: conn}
	channel := clientChannel(stream, authorityService)
	client, err := NewClient(context.Background(), channel, testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.isolatedDelivery = true
	channel.authority.checkPresent = func() error { return ErrAuthority }
	err = client.Move(context.Background(), 1, 0)
	if certainty, known := application.InjectionCertaintyOf(err); !known || certainty != application.InjectionNotApplied || stream.writes.Load() != 0 || client.sequence != 1 {
		t.Fatal("dead pinned owner received input", err, stream.writes.Load(), client.sequence)
	}
}
