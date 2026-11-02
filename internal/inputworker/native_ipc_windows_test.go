//go:build windows

package inputworker

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Exercise actual QPC timestamps through both codecs and the same portable
// exchange used by the installed native child. No Windows input is injected.
func TestNativeIPCSeparatesExecutorAndReplyDeliveryDelay(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	defer conn.Close()
	_ = server.SetDeadline(time.Now().Add(3 * time.Second))
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan error, 1)
	go func() {
		handler := PortableInputHandler(&recordingInput{}, func(_ context.Context, work func() error) error {
			time.Sleep(15 * time.Millisecond)
			return work()
		})
		// Client startup performs one heartbeat before the profiled input.
		for range 2 {
			if err := handler(server, ipcDelayedWriter{server}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	transport := &ipcDelayedTransport{conn}
	client, err := newPortableClient(context.Background(), transport)
	if err != nil {
		t.Fatal(err)
	}
	var detail handoff.WorkerApplyProfile
	ctx := handoff.WithWorkerApply(context.Background(), &detail)
	if err := client.Move(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if detail.NativeIPC == nil {
		t.Fatal("same-exchange IPC detail missing")
	}
	p := detail.NativeIPC
	if p.Micros[0] < 10_000 || p.Micros[3] < 10_000 || p.Micros[4] < 10_000 {
		t.Fatal("executor, child handler or reply-delivery delay misattributed", p)
	}
	if !p.ParentMeasured || p.ParentMicros[2] < 10_000 {
		t.Fatal("post-decode transport completion disappeared", p)
	}
}

type ipcDelayedTransport struct{ net.Conn }

func (p *ipcDelayedTransport) ExchangeInput(_ context.Context, exchange func(io.Writer, io.Reader) error) error {
	time.Sleep(15 * time.Millisecond)
	err := exchange(p.Conn, p.Conn)
	time.Sleep(15 * time.Millisecond)
	return err
}

type ipcDelayedWriter struct{ io.Writer }

func (w ipcDelayedWriter) Write(data []byte) (int, error) {
	time.Sleep(15 * time.Millisecond)
	return w.Writer.Write(data)
}
