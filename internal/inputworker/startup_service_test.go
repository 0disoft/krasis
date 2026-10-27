package inputworker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestServiceReadinessReceiptBoundedAndPreservesNextFrame(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		var p handoff.StartupProfile
		p.Record(handoff.ServiceLaunch, time.Millisecond)
		if prepared {
			p.Record(handoff.ServiceChildConnect, 0)
			p.Prepared, p.PreparedAgeMicros = true, 1_366_216
			p.PreparationDecision = handoff.PreparationUsed
			p.RecordPreparationWait(0)
		}
		p.ServiceReadiness.Record(handoff.ServiceLoginAdmission, 317*time.Millisecond)
		p.ServiceReadiness.Record(handoff.ServiceReadyAuthorization, 0)
		var wire bytes.Buffer
		if err := writeStartupReady(&wire, p); err != nil || wire.Len() != startupReadyServiceSize || string(wire.Bytes()[:4]) != "KRD7" {
			t.Fatal("service readiness envelope lost", wire.Len(), err)
		}
		packet := append([]byte(nil), wire.Bytes()...)
		check := func(data []byte, invalid bool) {
			t.Helper()
			reader := bytes.NewReader(append(append([]byte(nil), data...), 'S'))
			got, err := readStartupReady(iotest.OneByteReader(reader))
			if err != nil || (invalid && got != nil) || (!invalid && (got == nil || *got != p)) {
				t.Fatal("diagnostic changed valid readiness", got, err)
			}
			if next, err := reader.ReadByte(); err != nil || next != 'S' {
				t.Fatal("diagnostic consumed next protocol frame")
			}
		}
		check(packet, false)
		for length := startupReadyWaitSize; length < len(packet); length++ {
			if _, err := readStartupReady(bytes.NewReader(packet[:length])); !errors.Is(err, ErrProtocol) {
				t.Fatal("truncated service receipt accepted", length, err)
			}
		}
		for _, offset := range []int{startupReadyWaitSize + 1, startupReadyWaitSize + 7} {
			bad := append([]byte(nil), packet...)
			bad[offset] = 1
			if _, err := readStartupReady(bytes.NewReader(bad)); !errors.Is(err, ErrProtocol) {
				t.Fatal("reserved service byte accepted", offset)
			}
		}
		for _, mask := range []byte{0, 1 << handoff.ServiceReadinessStageCount, 1} {
			bad := append([]byte(nil), packet...)
			bad[startupReadyWaitSize] = mask
			check(bad, true) // Includes unmeasured nonzero admission time.
		}
		bad := append([]byte(nil), packet...)
		binary.BigEndian.PutUint32(bad[startupReadyWaitSize+8+int(handoff.ServiceLoginAdmission)*4:], uint32(handoff.Window.Microseconds()+1))
		check(bad, true)
		child := p
		child.ServiceReadiness.Record(handoff.ServiceLoginAdmission, 0)
		mergeWorkerStartup(&p, &child)
		if p.ServiceReadiness.Micros[handoff.ServiceLoginAdmission] != 317_000 {
			t.Fatal("child overwrote service-owned measurements")
		}
	}
}

func TestServiceReadinessIncludesFinalFreshProofBeforeReceipt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, pipe := net.Pipe()
		defer peer.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		var p handoff.StartupProfile
		p.Record(handoff.ServiceBootstrapReady, 74*time.Millisecond)
		p.ServiceReadiness.Record(handoff.ServiceLoginAdmission, 317*time.Millisecond)
		checks := 0
		owner.authority.checkInput = func() error { checks++; time.Sleep(25 * time.Millisecond); return nil }
		done := make(chan error, 1)
		go func() {
			got, err := readStartupReady(peer)
			if err == nil && (got == nil || got.ServiceReadiness.Micros[handoff.ServiceReadyAuthorization] != 25_000 || got.ServiceReadiness.Micros[handoff.ServiceLoginAdmission] != 317_000) {
				err = ErrProtocol
			}
			done <- err
		}()
		if err := publishStartupReady(context.Background(), owner, time.Now().Add(time.Minute), p); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil || checks != 1 {
			t.Fatal("service receipt lost or skipped fresh proof", checks, err)
		}
	})
}
