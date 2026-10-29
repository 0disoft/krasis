package inputworker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestNativeStartupRoundtripKeepsBothChildrenAndFollowingFrame(t *testing.T) {
	var p handoff.StartupProfile
	p.Record(handoff.WorkerDesktopReady, 150*time.Millisecond)
	p.Native = [2]handoff.NativeStartupProfile{{Measured: 31, Micros: [5]uint32{65000, 2000, 3000, 1000, 4000}}, {Measured: 31, Micros: [5]uint32{70000, 2500, 3500, 1500, 4500}}}
	var wire bytes.Buffer
	if err := writeStartupReady(&wire, p); err != nil {
		t.Fatal(err)
	}
	packet := append([]byte(nil), wire.Bytes()...)
	if len(packet) != startupReadyNativeSize || string(packet[:4]) != "KRD8" {
		t.Fatal("native profile not encoded")
	}
	reader := bytes.NewReader(append(packet, 'S'))
	got, err := readStartupReady(reader)
	if err != nil || got == nil || *got != p {
		t.Fatal("native details changed during framing", got, err)
	}
	if next, err := reader.ReadByte(); next != 'S' || err != nil {
		t.Fatal("native detail consumed next frame")
	}
	for n := startupReadyServiceSize; n < len(packet); n++ {
		if _, err := readStartupReady(bytes.NewReader(packet[:n])); !errors.Is(err, ErrProtocol) {
			t.Fatal("truncated native detail accepted", n, err)
		}
	}
	bad := append([]byte(nil), packet...)
	bad[startupReadyServiceSize+1] = 1
	if _, err := readStartupReady(bytes.NewReader(bad)); !errors.Is(err, ErrProtocol) {
		t.Fatal("reserved native byte accepted")
	}
	bad = append([]byte(nil), packet...)
	binary.BigEndian.PutUint32(bad[startupReadyServiceSize+4:], uint32(handoff.Window.Microseconds())+1)
	if got, err := readStartupReady(bytes.NewReader(bad)); got != nil || err != nil {
		t.Fatal("invalid numerical diagnostics changed readiness", got, err)
	}
	var merged handoff.StartupProfile
	merged.Record(handoff.ServiceLaunch, time.Millisecond)
	mergeWorkerStartup(&merged, &p)
	if merged.Native != p.Native || merged.Micros[handoff.ServiceLaunch] != 1000 {
		t.Fatal("native details lost in journal/service merge")
	}
}

type nativeStartupTransport struct {
	prefix   []byte
	calls    int
	deadline time.Time
	next     *entryTransport
	t        *testing.T
}

func (p *nativeStartupTransport) Close() error { return nil }
func (p *nativeStartupTransport) ExchangeInput(ctx context.Context, work func(io.Writer, io.Reader) error) error {
	p.calls++
	deadline, ok := ctx.Deadline()
	if !ok {
		p.t.Fatal("native startup lost deadline")
	}
	if p.calls == 1 {
		p.deadline = deadline
		time.Sleep(700 * time.Millisecond)
		var sent bytes.Buffer
		err := work(&sent, bytes.NewReader(p.prefix))
		if sent.Len() != 0 {
			p.t.Fatal("diagnostic receipt submitted input")
		}
		return err
	}
	if !deadline.Equal(p.deadline) {
		p.t.Fatal("heartbeat renewed the startup budget")
	}
	return p.next.ExchangeInput(ctx, work)
}

func TestNativeStartupReceiptNeverReplacesFreshHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, malformed := range []bool{false, true} {
			var p handoff.StartupProfile
			for _, stage := range []handoff.StartupStage{handoff.WorkerInstalledValidation, handoff.WorkerServiceAuth, handoff.WorkerAgentAuth, handoff.WorkerIdentity, handoff.WorkerDesktopReady} {
				p.Record(stage, time.Millisecond)
			}
			var wire bytes.Buffer
			_ = writeStartupReady(&wire, p)
			if malformed {
				wire.Reset()
				wire.WriteString("bad")
			}
			input := &portableFixture{}
			authorizations := 0
			transport := &nativeStartupTransport{prefix: wire.Bytes(), t: t, next: &entryTransport{handle: PortableInputHandler(input, func(_ context.Context, work func() error) error { authorizations++; return work() })}}
			client, profile, err := newNativePortableClient(context.Background(), transport)
			if malformed {
				if err == nil || client != nil || transport.calls != 1 || authorizations != 0 {
					t.Fatal("bad startup reached heartbeat")
				}
			} else if err != nil || client == nil || profile.Measured != 31 || transport.calls != 2 || authorizations != 1 || input.hits != 0 {
				t.Fatal("startup bypassed fresh heartbeat or injected input", err, profile)
			}
		}
	})
}
