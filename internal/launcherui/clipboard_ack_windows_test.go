//go:build windows

package launcherui

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestClipboardLostApplicationReceiptRetriesWithoutPayloadReplay(t *testing.T) {
	const id = "received-copy"
	// The native write already succeeded. Only its local-agent receipt is pending.
	pump := clipboardPump{sequence: 8, pendingAck: id}
	value := peerupdate.ClipboardValue{ID: id, Format: "files", Data: []byte("already extracted")}
	receipts, payloads := 0, 0
	send := func(request localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		if request.Ack != id {
			payloads++
			return localapi.ClipboardResult{Code: "ok", Value: value}, nil
		}
		receipts++
		if receipts <= 3 {
			return localapi.ClipboardResult{}, errors.New("injected lost application receipt")
		}
		return localapi.ClipboardResult{Code: "ok"}, nil
	}
	for attempt := 0; attempt < 4; attempt++ {
		request := localapi.ClipboardRequest{Action: "SYNC"}
		if attempt == 0 {
			request.Ack = id // Immediate receipt after the original native write.
		}
		result, err := pump.exchangeClipboard(context.Background(), 8, request, send)
		if result.Value.ID != "" || payloads != 0 || pump.sequence != 8 {
			t.Error("lost receipt replayed an already applied payload", result.Value.ID, payloads, pump)
		}
		if attempt < 3 && (err == nil || pump.pendingAck != id) {
			t.Error("failed receipt was consumed", err, pump)
		}
		if attempt == 3 && (err != nil || pump.pendingAck != "") {
			t.Error("successful receipt was not committed", err, pump)
		}
	}
	if receipts != 4 || payloads != 0 {
		t.Fatalf("receipt requests=%d repeated payloads=%d; want 4 and 0", receipts, payloads)
	}
}

func TestClipboardPendingReceiptYieldsToLocalChange(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		pump := clipboardPump{sequence: 9, pendingAck: "old-remote-copy", localChanged: true}
		text := "new local copy"
		_, _ = pump.exchangeClipboard(context.Background(), 8, localapi.ClipboardRequest{Action: "SYNC", LocalChanged: true, Text: &text}, func(request localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
			if request.Ack != "" {
				t.Fatal("receipt combined with a superseding local change")
			}
			if accepted {
				return localapi.ClipboardResult{Code: "ok"}, nil
			}
			return localapi.ClipboardResult{}, errors.New("injected local copy failure")
		})
		if accepted && (pump.pendingAck != "" || pump.localChanged || pump.sequence != 9) {
			t.Fatal("accepted local copy retained an obsolete receipt", pump)
		}
		if !accepted && (pump.pendingAck == "" || !pump.localChanged || pump.sequence != 8) {
			t.Fatal("failed local copy lost pending state", pump)
		}
	}
}

func TestClipboardReceiptDoesNotSuppressNextRemoteCopy(t *testing.T) {
	pump := clipboardPump{sequence: 8, pendingAck: "old-copy"}
	result, err := pump.exchangeClipboard(context.Background(), 8, localapi.ClipboardRequest{Action: "SYNC"}, func(request localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		if request.Ack != "old-copy" {
			t.Fatal("missing previous copy receipt")
		}
		return localapi.ClipboardResult{Code: "ok", Value: peerupdate.ClipboardValue{ID: "new-copy", Text: "new content"}}, nil
	})
	if err != nil || result.Value.ID != "new-copy" || pump.pendingAck != "" {
		t.Fatal("receipt retry hid a newer remote copy", result, err, pump)
	}
}
