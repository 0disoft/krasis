package linuxclipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Opt in on a disposable X11 display, for example:
// xvfb-run -a env KRASIS_X11_TEST=1 go test ./internal/linuxclipboard -run X11 -count=1
// Xvfb is test infrastructure only; the bridge never invokes external tools.
func testX11(t *testing.T) (*xClipboard, context.Context) {
	t.Helper()
	if os.Getenv("KRASIS_X11_TEST") != "1" {
		t.Skip("set KRASIS_X11_TEST=1 on a disposable Xvfb DISPLAY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	clip, err := openClipboard(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	x := clip.(*xClipboard)
	t.Cleanup(x.close)
	return x, ctx
}

func testClient(t *testing.T, ctx context.Context) *xClipboard {
	t.Helper()
	c, wire, err := connectX(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = wire.SetDeadline(time.Time{})
	t.Cleanup(func() { _ = wire.Close(); c.Close() })
	x := &xClipboard{c: c, wire: wire}
	return x
}

func waitXEvent(ctx context.Context, c *xgb.Conn, match func(xgb.Event) bool) error {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		for range 256 {
			e, err := c.PollForEvent()
			if err != nil {
				return err
			}
			if e == nil {
				break
			}
			if match(e) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func requestX(t *testing.T, ctx context.Context, source *xClipboard, a atoms, target xproto.Atom) (xproto.Atom, byte, []byte) {
	t.Helper()
	w, err := source.newWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = xproto.DestroyWindowChecked(source.c, w).Check() }()
	if err := xproto.ConvertSelectionChecked(source.c, w, a.clipboard, target, a.property, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	var notification xproto.SelectionNotifyEvent
	err = waitXEvent(ctx, source.c, func(e xgb.Event) bool {
		v, ok := e.(xproto.SelectionNotifyEvent)
		if ok && v.Requestor == w && v.Target == target {
			notification = v
			return true
		}
		return false
	})
	if err != nil {
		t.Fatal(err)
	}
	if notification.Property == 0 {
		return 0, 0, nil
	}
	property, err := xproto.GetProperty(source.c, false, w, a.property, xproto.GetPropertyTypeAny, 0, (peerupdate.MaxClipboardImageBytes+3)/4).Reply()
	if err != nil || property == nil || property.BytesAfter != 0 {
		t.Fatal("property fetch", err)
	}
	if property.Type != a.incr {
		return property.Type, property.Format, property.Value
	}
	if property.Format != 32 || len(property.Value) != 4 {
		t.Fatal("invalid INCR header")
	}
	advertised := int(xgb.Get32(property.Value))
	if advertised <= 0 || advertised > peerupdate.MaxClipboardImageBytes {
		t.Fatal("unbounded INCR")
	}
	if err := xproto.DeletePropertyChecked(source.c, w, a.property).Check(); err != nil {
		t.Fatal(err)
	}
	var result []byte
	for {
		err = waitXEvent(ctx, source.c, func(e xgb.Event) bool {
			v, ok := e.(xproto.PropertyNotifyEvent)
			return ok && v.Window == w && v.Atom == a.property && v.State == xproto.PropertyNewValue
		})
		if err != nil {
			t.Fatal(err)
		}
		p, err := xproto.GetProperty(source.c, true, w, a.property, xproto.GetPropertyTypeAny, 0, 32<<10).Reply()
		if err != nil || p == nil || p.BytesAfter != 0 || p.Type != target || p.Format != 8 {
			t.Fatal("invalid INCR chunk", err)
		}
		result = append(result, p.Value...)
		if len(result) > advertised {
			t.Fatal("INCR exceeded advertised bytes")
		}
		if len(p.Value) == 0 {
			break
		}
	}
	if len(result) != advertised {
		t.Fatal("INCR was truncated")
	}
	return target, 8, result
}

func TestX11ServesUTF8TargetsAndLegacyProperty(t *testing.T) {
	x, ctx := testX11(t)
	o, err := x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	text := "제로디 🐧 café"
	if _, err := x.write(ctx, o.revision, peerupdate.ClipboardValue{ID: testReceipt, Text: text}); err != nil {
		t.Fatal(err)
	}
	r := testClient(t, ctx)
	typeAtom, format, data := requestX(t, ctx, r, x.a, x.a.targets)
	if typeAtom != xproto.AtomAtom || format != 32 || len(data) < 12 {
		t.Fatal("TARGETS not served")
	}
	for _, target := range []xproto.Atom{x.a.utf8, x.a.plain, x.a.text} {
		kind, format, data := requestX(t, ctx, r, x.a, target)
		if kind != x.a.utf8 && kind != x.a.plain || format != 8 || string(data) != text {
			t.Fatalf("UTF8 conversion failed: %d/%d/%q", kind, format, data)
		}
	}
	kind, _, _ := requestX(t, ctx, r, x.a, xproto.AtomString)
	if kind != 0 {
		t.Fatal("non-Latin1 text advertised as STRING")
	}
	// Property=None must use the target atom, not fail the old-client request.
	w, err := r.newWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = xproto.DestroyWindowChecked(r.c, w).Check() }()
	if err := xproto.ConvertSelectionChecked(r.c, w, x.a.clipboard, x.a.utf8, 0, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	if err := waitXEvent(ctx, r.c, func(e xgb.Event) bool {
		v, ok := e.(xproto.SelectionNotifyEvent)
		return ok && v.Requestor == w && v.Property == x.a.utf8
	}); err != nil {
		t.Fatal(err)
	}
}

func noisePNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 256, 256))
	var state uint32 = 0x12345678
	for i := range im.Pix {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		im.Pix[i] = byte(state)
	}
	var data bytes.Buffer
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestX11ServesPNGViaBoundedINCR(t *testing.T) {
	x, ctx := testX11(t)
	data := noisePNG(t)
	if len(data) <= x.chunk {
		t.Fatal("fixture must exercise INCR")
	}
	o, err := x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.write(ctx, o.revision, peerupdate.ClipboardValue{ID: testReceipt, Format: "png", Data: data}); err != nil {
		t.Fatal(err)
	}
	r := testClient(t, ctx)
	kind, format, got := requestX(t, ctx, r, x.a, x.a.png)
	if kind != x.a.png || format != 8 || !bytes.Equal(got, data) {
		t.Fatal("PNG INCR roundtrip changed bytes")
	}
}

type selectionSource struct {
	x             *xClipboard
	mu            sync.Mutex
	value         peerupdate.ClipboardValue
	mode          string
	lastRequestor xproto.Window
}

func newSelectionSource(t *testing.T, ctx context.Context, a atoms) *selectionSource {
	t.Helper()
	x := testClient(t, ctx)
	x.a = a
	w, err := x.newWindow()
	if err != nil {
		t.Fatal(err)
	}
	x.window = w
	x.owner, x.chunk, x.transfers = w, 16<<10, make(map[transferKey]*outgoing)
	s := &selectionSource{x: x}
	done := make(chan struct{})
	workerCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(done)
		_ = waitXEvent(workerCtx, x.c, func(event xgb.Event) bool {
			if e, ok := event.(xproto.PropertyNotifyEvent); ok {
				x.advance(e)
				return false
			}
			if e, ok := event.(xproto.DestroyNotifyEvent); ok {
				x.dropWindow(e.Window)
				return false
			}
			e, ok := event.(xproto.SelectionRequestEvent)
			if !ok {
				return false
			}
			s.mu.Lock()
			v, mode := s.value, s.mode
			s.lastRequestor = e.Requestor
			s.mu.Unlock()
			if mode == "silent" {
				return false
			}
			if mode == "actualincr" {
				v.ID = testReceipt
				x.value = v
				x.serve(e)
				return false
			}
			property := e.Property
			if property == 0 {
				property = e.Target
			}
			notify := xproto.SelectionNotifyEvent{Time: e.Time, Requestor: e.Requestor, Selection: e.Selection, Target: e.Target}
			target, format, data := e.Target, byte(8), []byte(v.Text)
			if e.Target == a.targets {
				target, format = xproto.AtomAtom, 32
				available := a.utf8
				if mode == "latin1" {
					available = xproto.AtomString
				}
				if v.Format == "png" {
					available = a.png
				}
				data = make([]byte, 4)
				xgb.Put32(data, uint32(available))
			} else if mode == "incr" {
				target, format = a.incr, 32
				data = make([]byte, 4)
				xgb.Put32(data, 100)
			} else if mode == "oversizeincr" {
				target, format = a.incr, 32
				data = make([]byte, 4)
				xgb.Put32(data, ^uint32(0))
			} else if mode == "understated" {
				_ = xproto.ChangeWindowAttributesChecked(x.c, e.Requestor, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange | xproto.EventMaskStructureNotify}).Check()
				now := time.Now()
				x.transfers[transferKey{e.Requestor, property}] = &outgoing{target: e.Target, data: []byte(v.Text), deadline: now.Add(5 * time.Second), expires: now.Add(10 * time.Second)}
				target, format = a.incr, 32
				data = make([]byte, 4)
				xgb.Put32(data, 1)
			} else if v.Format == "png" && e.Target == a.png {
				data = v.Data
			} else if mode == "latin1" && e.Target == xproto.AtomString {
				data = []byte{'c', 'a', 'f', 0xe9}
			} else if e.Target != a.utf8 {
				x.sendNotify(notify)
				return false
			}
			if err := xproto.ChangePropertyChecked(x.c, xproto.PropModeReplace, e.Requestor, property, target, format, uint32(len(data)/(int(format)/8)), data).Check(); err == nil {
				notify.Property = property
			}
			x.sendNotify(notify)
			return false
		})
	}()
	t.Cleanup(func() { cancel(); <-done })
	return s
}

func (s *selectionSource) offer(t *testing.T, value peerupdate.ClipboardValue, mode string) {
	t.Helper()
	s.mu.Lock()
	s.value, s.mode = value, mode
	s.mu.Unlock()
	if err := xproto.SetSelectionOwnerChecked(s.x.c, s.x.window, s.x.a.clipboard, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
}

func TestX11ReadsRepeatedOwnerUTF8Latin1AndPNG(t *testing.T) {
	x, ctx := testX11(t)
	s := newSelectionSource(t, ctx, x.a)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	last := uint64(0)
	for _, tc := range []struct {
		value peerupdate.ClipboardValue
		mode  string
	}{
		{peerupdate.ClipboardValue{Text: "제로디 🐧"}, ""},
		{peerupdate.ClipboardValue{Text: "same owner second copy"}, ""},
		{peerupdate.ClipboardValue{Text: "café"}, "latin1"},
		{peerupdate.ClipboardValue{Format: "png", Data: pngData.Bytes()}, ""},
	} {
		s.offer(t, tc.value, tc.mode)
		o, err := x.observe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if o.revision <= last {
			t.Fatal("same-owner copy not observed")
		}
		last = o.revision
		got, err := x.read(ctx, o.revision)
		if err != nil || got.Format != tc.value.Format || got.Text != tc.value.Text || !bytes.Equal(got.Data, tc.value.Data) {
			t.Fatalf("conversion failed: %s %v", tc.mode, err)
		}
	}
}

func TestX11RejectsOversizedINCRDestroysRequestorAndRecovers(t *testing.T) {
	x, ctx := testX11(t)
	s := newSelectionSource(t, ctx, x.a)
	s.offer(t, peerupdate.ClipboardValue{Text: "large"}, "oversizeincr")
	o, err := x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.read(ctx, o.revision); !errors.Is(err, ErrTooLarge) {
		t.Fatal("INCR not rejected", err)
	}
	s.mu.Lock()
	requestor := s.lastRequestor
	s.mu.Unlock()
	if requestor == 0 {
		t.Fatal("missing conversion request")
	}
	if _, err := xproto.GetWindowAttributes(s.x.c, requestor).Reply(); err == nil {
		t.Fatal("INCR requestor not destroyed")
	}
	s.offer(t, peerupdate.ClipboardValue{Text: "small retry"}, "")
	o, err = x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	v, err := x.read(ctx, o.revision)
	if err != nil || v.Text != "small retry" {
		t.Fatal("INCR poisoned next conversion", err)
	}
}

func TestX11ConversionTimeoutAndCanceledContext(t *testing.T) {
	x, ctx := testX11(t)
	s := newSelectionSource(t, ctx, x.a)
	s.offer(t, peerupdate.ClipboardValue{Text: "silent"}, "silent")
	o, err := x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := x.read(ctx, o.revision); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("conversion timeout unbounded")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := x.read(canceled, o.revision); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s.offer(t, peerupdate.ClipboardValue{Text: "recovered"}, "")
	o, err = x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := x.read(ctx, o.revision); err != nil || v.Text != "recovered" {
		t.Fatal("timeout poisoned next conversion", err)
	}
}

func TestX11ReceivesPNGAndTextINCR(t *testing.T) {
	x, ctx := testX11(t)
	s := newSelectionSource(t, ctx, x.a)
	for _, value := range []peerupdate.ClipboardValue{
		{Text: string(bytes.Repeat([]byte("abcd"), 12000))},
		{Format: "png", Data: noisePNG(t)},
	} {
		s.offer(t, value, "actualincr")
		o, err := x.observe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		v, err := x.read(ctx, o.revision)
		if err != nil || v.Text != value.Text || v.Format != value.Format || !bytes.Equal(v.Data, value.Data) {
			t.Fatal("INCR receive failed", value.Format, err)
		}
	}
}

func TestX11RunCancellationAndTransientErrors(t *testing.T) {
	if os.Getenv("KRASIS_X11_TEST") != "1" {
		t.Skip("requires disposable Xvfb DISPLAY")
	}
	ctx, cancel := context.WithCancel(context.Background())
	errorsSeen := make(chan error, 1)
	done := make(chan error, 1)
	control := t.TempDir()
	go func() {
		done <- Run(ctx, Options{ControlDirectory: control, PollInterval: 10 * time.Millisecond, OnError: func(err error) {
			select {
			case errorsSeen <- err:
			default:
			}
		}})
	}()
	select {
	case err := <-errorsSeen:
		if err == nil {
			t.Fatal("missing transient error")
		}
	case err := <-done:
		t.Fatal("bridge exited before cancellation", err)
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("OnError not called")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not close X11")
	}
}

func TestX11INCRCumulativeLimitAndActiveCancellation(t *testing.T) {
	x, ctx := testX11(t)
	s := newSelectionSource(t, ctx, x.a)
	s.offer(t, peerupdate.ClipboardValue{Text: strings.Repeat("x", peerupdate.MaxClipboardBytes+1)}, "understated")
	o, err := x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.read(ctx, o.revision); !errors.Is(err, ErrTooLarge) {
		t.Fatal("understated INCR escaped cumulative limit", err)
	}
	s.offer(t, peerupdate.ClipboardValue{Text: "never replies"}, "silent")
	o, err = x.observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := x.read(short, o.revision); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("active read cancellation was unbounded")
	}
}

func TestX11SingleBridgeAndHandshakeCancellation(t *testing.T) {
	_, ctx := testX11(t)
	if second, err := openClipboard(ctx, ""); !errors.Is(err, ErrAlreadyRunning) {
		if second != nil {
			second.close()
		}
		t.Fatal("duplicate bridge accepted", err)
	}
	number := 59431 + os.Getpid()%500
	path := fmt.Sprintf("/tmp/.X11-unix/X%d", number)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
		close(accepted)
	}()
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := connectX(short, fmt.Sprintf(":%d", number)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("handshake cancellation lost", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("handshake cancellation unbounded")
	}
	select {
	case conn := <-accepted:
		if conn != nil {
			conn.Close()
		}
	case <-time.After(time.Second):
		t.Fatal("X11 listener not reached")
	}
}

func TestX11OpenRejectsWaylandAndRemoteDisplays(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if _, err := openClipboard(context.Background(), ":91"); !errors.Is(err, ErrWayland) {
		t.Fatal(err)
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XDG_SESSION_TYPE", "x11")
	if _, err := openClipboard(context.Background(), "localhost:91"); err == nil {
		t.Fatal("remote X11 accepted")
	}
	start := time.Now()
	if _, err := openClipboard(context.Background(), ":59535"); err == nil {
		t.Fatal("missing X server accepted")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("open was unbounded")
	}
}
