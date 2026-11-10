// Package linuxclipboard bridges an X11 CLIPBOARD selection to Krasis's
// authenticated local API. Sharing must already be enabled by the CLI/UI.
package linuxclipboard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

var (
	errXClosed        = errors.New("X11 clipboard connection closed")
	ErrAlreadyRunning = errors.New("an X11 clipboard bridge already owns this display")
	ErrWayland        = errors.New("clipboard bridge requires an X11 session; native Wayland is unsupported")
	ErrUnsupported    = errors.New("unsupported X11 clipboard format (text and PNG only)")
	ErrTooLarge       = errors.New("X11 clipboard exceeds format limit")
	ErrChanged        = errors.New("X11 clipboard changed during exchange")
)

// Options configures Run. Run never enables clipboard sharing itself. OnError
// receives transient failures without clipboard contents or credential values.
type Options struct {
	ControlDirectory string
	Display          string        // Empty uses DISPLAY. Requires X11 and XFIXES; not native Wayland.
	PollInterval     time.Duration // Default 250ms, maximum 1s (agent heartbeat).
	OnError          func(error)
}

type observation struct {
	revision uint64
	owned    bool
}
type clipboard interface {
	observe(context.Context) (observation, error)
	read(context.Context, uint64) (peerupdate.ClipboardValue, error)
	write(context.Context, uint64, peerupdate.ClipboardValue) (observation, error)
	close()
}
type api interface {
	Update(context.Context, localapi.UpdateRequest) (localapi.UpdateResult, error)
	Clipboard(context.Context, localapi.ClipboardRequest) (localapi.ClipboardResult, error)
}

// Run blocks until cancellation or an X11 failure. Call it in a goroutine in the
// Linux CLI wrapper, passing the same control directory as the resident agent.
// The caller owns cancellation and may retry Run after an agent/display restart.
func Run(ctx context.Context, options Options) error {
	if ctx == nil {
		return errors.New("clipboard context is required")
	}
	if options.ControlDirectory == "" {
		return errors.New("clipboard control directory is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	native, err := openClipboard(ctx, options.Display)
	if err != nil {
		return err
	}
	defer native.close()
	interval := options.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	if interval > time.Second {
		interval = time.Second
	}
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	p := pump{}
	for {
		// Reopen credentials on every tick so an agent restart cannot strand the
		// bridge on an obsolete endpoint or bearer token.
		client, err := localapi.OpenClient(options.ControlDirectory)
		if err == nil {
			stepCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err = p.step(stepCtx, client, native)
			cancel()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errXClosed) {
			return err
		}
		if err != nil && options.OnError != nil {
			options.OnError(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type pump struct {
	peer       string
	baseline   bool
	revision   uint64
	pendingAck string
	failedID   string
}

func (p *pump) step(ctx context.Context, client api, native clipboard) error {
	status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil {
		return err
	}
	if status.Code != "ok" {
		return fmt.Errorf("clipboard status: %s", status.Code)
	}
	peer := status.Status.ClipboardPeer
	if peer == "" {
		*p = pump{}
		return nil
	}
	if !(localapi.ClipboardRequest{Action: "SYNC", Peer: peer}).Valid() {
		return errors.New("invalid clipboard peer")
	}
	current, err := native.observe(ctx)
	if err != nil {
		return err
	}
	if peer != p.peer || !p.baseline {
		// Existing clipboard contents are private until the next local copy.
		*p = pump{peer: peer, baseline: true, revision: current.revision}
	}
	request := localapi.ClipboardRequest{Action: "SYNC", Peer: peer}
	changed := current.revision != p.revision && !current.owned
	if changed {
		p.pendingAck = ""
		p.failedID = ""
		request.LocalChanged = true
		value, readErr := native.read(ctx, current.revision)
		if readErr == nil {
			if value.Format == "" {
				request.Text = &value.Text
			} else {
				request.Format, request.Data = value.Format, value.Data
			}
		} else if !errors.Is(readErr, ErrUnsupported) && !errors.Is(readErr, ErrTooLarge) {
			return readErr
		}
	} else {
		request.Ack = p.pendingAck
	}
	result, err := client.Clipboard(ctx, request)
	if err != nil {
		return err
	} // Retry the latest selection, never retained payload bytes.
	if result.Code != "ok" {
		return fmt.Errorf("clipboard sync: %s", result.Code)
	}
	p.revision, p.pendingAck = current.revision, ""
	if changed || result.Value.ID == "" || result.Value.ID == p.failedID {
		return nil
	}
	if !validReceipt(result.Value.ID) {
		return errors.New("invalid clipboard receipt")
	}
	if err = validateValue(result.Value); err == nil {
		var installed observation
		installed, err = native.write(ctx, current.revision, result.Value)
		if err == nil {
			p.revision, p.pendingAck = installed.revision, result.Value.ID
			// Do not apply again if the ACK response is lost. The next tick only
			// retries the receipt, unless a newer local copy supersedes it.
			ack, ackErr := client.Clipboard(ctx, localapi.ClipboardRequest{Action: "SYNC", Peer: peer, Ack: p.pendingAck})
			if ackErr == nil && ack.Code == "ok" {
				p.pendingAck = ""
			}
			return ackErr
		}
	}
	if errors.Is(err, ErrChanged) {
		return err
	}
	p.failedID = result.Value.ID
	_, reportErr := client.Clipboard(ctx, localapi.ClipboardRequest{Action: "REPORT", Peer: peer, Ack: result.Value.ID, Format: result.Value.Format, Report: "apply_failed"})
	return errors.Join(err, reportErr)
}

func validReceipt(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validateValue(v peerupdate.ClipboardValue) error {
	switch v.Format {
	case "":
		if len(v.Text) > peerupdate.MaxClipboardBytes {
			return ErrTooLarge
		}
		if len(v.Data) != 0 || !utf8.ValidString(v.Text) || strings.ContainsRune(v.Text, 0) {
			return ErrUnsupported
		}
	case "png":
		if len(v.Data) > peerupdate.MaxClipboardImageBytes {
			return ErrTooLarge
		}
		if v.Text != "" || !peerupdate.ValidClipboardImage("png", v.Data) {
			return ErrUnsupported
		}
	default:
		return ErrUnsupported
	}
	return nil
}
