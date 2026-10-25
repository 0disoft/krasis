package peerupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClipboardCancelledMediaWaitDoesNotNeedOwnerToFinish(t *testing.T) {
	m := testManager(t)
	m.generation = 72
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, changed := m.ClipboardChanges()
	m.clipboard.prepareMu.Lock()
	locked := true
	defer func() {
		if locked {
			m.clipboard.prepareMu.Unlock()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	local := &ClipboardValue{Format: "files", Data: testClipboardZip(t, "copy.txt", false, "private copy")}
	go func() {
		done <- m.queueClipboardLocal(ctx, peer, local, func(local *ClipboardValue) (*ClipboardValue, error) {
			t.Error("cancelled waiter reached payload cloning/validation")
			return prepareClipboardLocal(local)
		})
	}()
	<-changed // The newer local copy has reserved its epoch; media still owns the gate.
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Error("missing cancellation", err)
		}
	case <-time.After(time.Second):
		t.Error("cancelled request is still waiting for unrelated media preparation")
		m.clipboard.prepareMu.Unlock()
		locked = false
		<-done // Always join the regression fixture, including on the old implementation.
	}
	if m.clipboard.out != nil {
		t.Fatal("cancelled copy was published")
	}
}

func TestClipboardCancellationDuringPreparationCannotPublish(t *testing.T) {
	for _, format := range []string{"", "files"} {
		t.Run("format="+format, func(t *testing.T) {
			m := testManager(t)
			m.generation = 72
			peer := strings.Repeat("a", 64)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			local := &ClipboardValue{Text: "private copy"}
			if format == "files" {
				local = &ClipboardValue{Format: format, Data: testClipboardZip(t, "copy.txt", false, "private copy")}
			}
			err := m.queueClipboardLocal(ctx, peer, local, func(local *ClipboardValue) (*ClipboardValue, error) {
				value, err := prepareClipboardLocal(local)
				cancel() // Valid preparation can finish after the caller has left.
				return value, err
			})
			if !errors.Is(err, context.Canceled) || m.clipboard.out != nil || m.clipboard.transfer.State == "QUEUED" {
				t.Fatal("cancelled preparation advertised content", err)
			}
		})
	}
}

func TestClipboardExpiredRequestCannotClearAnExistingCopy(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	text := "current copy"
	if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
		t.Fatal(err)
	}
	epoch, id := m.clipboard.localEpoch, m.clipboard.out.ID
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	for _, local := range []*ClipboardValue{nil, {Text: "obsolete request"}} {
		if _, err := m.SyncClipboardLocalChange(ctx, peer, local, "", true); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("expired request accepted", err)
		}
		if m.clipboard.localEpoch != epoch || m.clipboard.out == nil || m.clipboard.out.ID != id {
			t.Fatal("expired request discarded or replaced the current copy")
		}
	}
}
