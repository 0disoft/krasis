//go:build windows

package launcherui

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestClipboardExchangeRetainsUnacknowledgedCopy(t *testing.T) {
	for _, mode := range []string{"transport", "rejected", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			pump := clipboardPump{sequence: 8, localChanged: true}
			text := "new local copy"
			request := localapi.ClipboardRequest{Action: "SYNC", Text: &text, LocalChanged: true}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			calls := 0
			_, _ = pump.exchangeClipboard(ctx, 7, request, func(localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
				calls++
				if mode == "transport" {
					return localapi.ClipboardResult{}, errors.New("injected lost API response")
				}
				return localapi.ClipboardResult{Code: "rejected"}, nil
			})
			wantCalls := 1
			if mode == "cancelled" {
				wantCalls = 0
			}
			if pump.sequence != 7 || !pump.localChanged || calls != wantCalls {
				t.Fatalf("unacknowledged copy consumed: sequence=%d changed=%t calls=%d", pump.sequence, pump.localChanged, calls)
			}
			// The next tick detects the same native sequence as a new local copy.
			pump.sequence = 8
			result, err := pump.exchangeClipboard(context.Background(), 7, request, func(got localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
				if got.Text == nil || *got.Text != text || !got.LocalChanged {
					t.Fatal("retry lost the copied contents or local-change marker")
				}
				return localapi.ClipboardResult{Code: "ok"}, nil
			})
			if err != nil || result.Code != "ok" || pump.sequence != 8 || pump.localChanged {
				t.Fatal("acknowledged retry did not commit the observation", result, err, pump)
			}
		})
	}
}

func TestClipboardExchangePreservesPayloadAndMetadataBoundaries(t *testing.T) {
	empty := ""
	for _, test := range []struct {
		name      string
		request   localapi.ClipboardRequest
		rollback  bool
		cancelled bool
	}{
		{"empty text", localapi.ClipboardRequest{Text: &empty}, true, false},
		{"image", localapi.ClipboardRequest{Format: "png", Data: []byte("image")}, true, false},
		{"files", localapi.ClipboardRequest{Format: "files", Data: []byte("archive")}, true, false},
		{"unshareable change", localapi.ClipboardRequest{LocalChanged: true}, false, false},
		{"receipt", localapi.ClipboardRequest{Ack: "received-copy"}, false, false},
		{"cancelled preparation", localapi.ClipboardRequest{LocalChanged: true}, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pump := clipboardPump{sequence: 8, localChanged: true}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancelled {
				cancel()
			}
			_, _ = pump.exchangeClipboard(ctx, 7, test.request, func(localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
				return localapi.ClipboardResult{}, errors.New("injected API failure")
			})
			want := uintptr(8)
			if test.rollback {
				want = 7
			}
			if pump.sequence != want || !pump.localChanged {
				t.Fatal("failure changed the wrong observation", pump)
			}
		})
	}
}
