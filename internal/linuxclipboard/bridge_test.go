package linuxclipboard

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

var testPeer = strings.Repeat("a", 64)
var testReceipt = strings.Repeat("b", 32)

type fakeClipboard struct {
	current           observation
	value             peerupdate.ClipboardValue
	readErr, writeErr error
	reads, writes     int
}

func (f *fakeClipboard) observe(context.Context) (observation, error) { return f.current, nil }
func (f *fakeClipboard) read(context.Context, uint64) (peerupdate.ClipboardValue, error) {
	f.reads++
	return f.value, f.readErr
}
func (f *fakeClipboard) write(_ context.Context, revision uint64, value peerupdate.ClipboardValue) (observation, error) {
	if f.writeErr != nil {
		return observation{}, f.writeErr
	}
	if revision != f.current.revision {
		return observation{}, ErrChanged
	}
	f.writes++
	f.value = value
	f.current = observation{revision + 1, true}
	return f.current, nil
}
func (*fakeClipboard) close() {}

type fakeAPI struct {
	peer     string
	requests []localapi.ClipboardRequest
	send     func(localapi.ClipboardRequest) (localapi.ClipboardResult, error)
}

func (f *fakeAPI) Update(ctx context.Context, _ localapi.UpdateRequest) (localapi.UpdateResult, error) {
	return localapi.UpdateResult{Code: "ok", Status: peerupdate.Status{ClipboardPeer: f.peer}}, ctx.Err()
}
func (f *fakeAPI) Clipboard(ctx context.Context, r localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
	if err := ctx.Err(); err != nil {
		return localapi.ClipboardResult{}, err
	}
	f.requests = append(f.requests, r)
	if !r.Valid() {
		return localapi.ClipboardResult{}, errors.New("invalid request")
	}
	if f.send != nil {
		return f.send(r)
	}
	return localapi.ClipboardResult{Code: "ok"}, nil
}

func setupPump(t *testing.T) (*pump, *fakeAPI, *fakeClipboard) {
	t.Helper()
	p, a, n := &pump{}, &fakeAPI{peer: testPeer}, &fakeClipboard{current: observation{revision: 1}, value: peerupdate.ClipboardValue{Text: "private old copy"}}
	if err := p.step(context.Background(), a, n); err != nil {
		t.Fatal(err)
	}
	if n.reads != 0 || len(a.requests) != 1 || a.requests[0].Text != nil {
		t.Fatal("existing contents were exported")
	}
	a.requests = nil
	return p, a, n
}

func TestAutomaticUTF8AndUnshareableChanges(t *testing.T) {
	for _, failure := range []error{nil, ErrUnsupported, ErrTooLarge} {
		t.Run(strings.ReplaceAll(strings.TrimSpace(errorText(failure)), " ", "_"), func(t *testing.T) {
			p, a, n := setupPump(t)
			n.current.revision++
			n.value.Text = "제로디 🐧"
			n.readErr = failure
			if err := p.step(context.Background(), a, n); err != nil {
				t.Fatal(err)
			}
			r := a.requests[0]
			if !r.LocalChanged || r.Ack != "" || failure == nil && (r.Text == nil || *r.Text != n.value.Text) || failure != nil && r.Text != nil {
				t.Fatalf("wrong change: %+v", r)
			}
			if err := p.step(context.Background(), a, n); err != nil {
				t.Fatal(err)
			}
			if n.reads != 1 || a.requests[1].LocalChanged {
				t.Fatal("same selection resent")
			}
		})
	}
}
func errorText(err error) string {
	if err == nil {
		return "UTF8"
	}
	return err.Error()
}

func TestFailedSendRereadsLatestSelection(t *testing.T) {
	p, a, n := setupPump(t)
	n.current.revision++
	n.value.Text = "first"
	a.send = func(localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		return localapi.ClipboardResult{}, errors.New("lost response")
	}
	if err := p.step(context.Background(), a, n); err == nil {
		t.Fatal("lost response ignored")
	}
	n.current.revision++
	n.value.Text = "newer"
	a.send = nil
	if err := p.step(context.Background(), a, n); err != nil {
		t.Fatal(err)
	}
	if got := a.requests[1].Text; got == nil || *got != "newer" {
		t.Fatal("replayed obsolete prepared contents")
	}
}

func TestIncomingApplyACKRetryAndNoEcho(t *testing.T) {
	p, a, n := setupPump(t)
	acks := 0
	a.send = func(r localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		if r.Ack != "" {
			acks++
			if acks == 1 {
				return localapi.ClipboardResult{}, errors.New("ACK response lost")
			}
			return localapi.ClipboardResult{Code: "ok"}, nil
		}
		return localapi.ClipboardResult{Code: "ok", Value: peerupdate.ClipboardValue{ID: testReceipt, Text: "remote"}}, nil
	}
	if err := p.step(context.Background(), a, n); err == nil {
		t.Fatal("ACK failure ignored")
	}
	if n.writes != 1 || p.pendingAck != testReceipt {
		t.Fatal("receipt not retained")
	}
	if err := p.step(context.Background(), a, n); err != nil {
		t.Fatal(err)
	}
	if n.writes != 1 || n.reads != 0 || acks != 2 || p.pendingAck != "" {
		t.Fatal("duplicate apply or reflected copy")
	}
}

func TestNewLocalCopySupersedesPendingACK(t *testing.T) {
	p, a, n := setupPump(t)
	p.pendingAck = testReceipt
	n.current.revision++
	n.value.Text = "local wins"
	if err := p.step(context.Background(), a, n); err != nil {
		t.Fatal(err)
	}
	if a.requests[0].Ack != "" || !a.requests[0].LocalChanged {
		t.Fatal("obsolete ACK hid local copy")
	}
}

func TestIncomingDoesNotOverwriteCopyDuringHTTP(t *testing.T) {
	p, a, n := setupPump(t)
	a.send = func(localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		n.current.revision++
		return localapi.ClipboardResult{Code: "ok", Value: peerupdate.ClipboardValue{ID: testReceipt, Text: "remote"}}, nil
	}
	if err := p.step(context.Background(), a, n); !errors.Is(err, ErrChanged) {
		t.Fatal(err)
	}
	if n.writes != 0 || len(a.requests) != 1 {
		t.Fatal("new local copy overwritten or acknowledged")
	}
}

func TestPNGAndUnsupportedIncoming(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"png", "dib", "files"} {
		t.Run(format, func(t *testing.T) {
			p, a, n := setupPump(t)
			a.send = func(r localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
				if r.Ack != "" || r.Action == "REPORT" {
					return localapi.ClipboardResult{Code: "ok"}, nil
				}
				return localapi.ClipboardResult{Code: "ok", Value: peerupdate.ClipboardValue{ID: testReceipt, Format: format, Data: pngData.Bytes()}}, nil
			}
			err := p.step(context.Background(), a, n)
			if format == "png" {
				if err != nil || n.writes != 1 || a.requests[1].Ack != testReceipt {
					t.Fatal("PNG not applied and acknowledged", err)
				}
			} else {
				if !errors.Is(err, ErrUnsupported) || n.writes != 0 || a.requests[1].Report != "apply_failed" {
					t.Fatal("unsupported content accepted", err)
				}
			}
		})
	}
}

func TestDisabledOrSwitchedPeerDoesNotExportOldClipboard(t *testing.T) {
	p, a, n := setupPump(t)
	a.peer = ""
	n.current.revision++
	if err := p.step(context.Background(), a, n); err != nil {
		t.Fatal(err)
	}
	if len(a.requests) != 0 || n.reads != 0 || p.baseline {
		t.Fatal("disabled bridge accessed clipboard")
	}
	a.peer = strings.Repeat("c", 64)
	if err := p.step(context.Background(), a, n); err != nil {
		t.Fatal(err)
	}
	if n.reads != 0 || a.requests[0].LocalChanged {
		t.Fatal("peer switch exported old copy")
	}
}

func TestPayloadLimitsAndEncoding(t *testing.T) {
	for _, tc := range []struct {
		value peerupdate.ClipboardValue
		want  error
	}{
		{peerupdate.ClipboardValue{Text: strings.Repeat("x", peerupdate.MaxClipboardBytes)}, nil},
		{peerupdate.ClipboardValue{Text: strings.Repeat("x", peerupdate.MaxClipboardBytes+1)}, ErrTooLarge},
		{peerupdate.ClipboardValue{Text: "\x00"}, ErrUnsupported},
		{peerupdate.ClipboardValue{Text: "\xff"}, ErrUnsupported},
		{peerupdate.ClipboardValue{Text: "ok", Data: []byte("mixed")}, ErrUnsupported},
		{peerupdate.ClipboardValue{Format: "png", Data: []byte("not PNG")}, ErrUnsupported},
		{peerupdate.ClipboardValue{Format: "files"}, ErrUnsupported},
	} {
		if got := validateValue(tc.value); !errors.Is(got, tc.want) {
			t.Fatalf("got %v, want %v", got, tc.want)
		}
	}
}

func TestCanceledRunNeedsNoDisplay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, Options{ControlDirectory: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
