package peerupdate

import (
	"io"
	"strings"
	"testing"
)

func TestUnshareableLocalCopyDiscardsPendingAndInFlightRemoteCopy(t *testing.T) {
	m := testManager(t)
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboard(peer, nil, "")
	if err := m.clipboardReceive(peer, id, strings.NewReader("remote old"), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if v, err := m.SyncClipboardLocalChange(peer, nil, "", true); err != nil || v.ID != "" {
		t.Fatal("older pending copy returned", v, err)
	}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		done <- m.clipboardReceive(peer, strings.Repeat("c", 32), &signalClipboardReader{Reader: reader, started: started}, func() bool { return true })
	}()
	<-started
	_, _ = m.SyncClipboardLocalChange(peer, nil, "", true)
	_, _ = writer.Write([]byte("remote late"))
	writer.Close()
	if err := <-done; err == nil {
		t.Fatal("late receive overwrote newer unshared local copy")
	}
	if v, _ := m.SyncClipboard(peer, nil, ""); v.ID != "" {
		t.Fatal(v)
	}
}

type signalClipboardReader struct {
	io.Reader
	started chan struct{}
	once    bool
}

func (r *signalClipboardReader) Read(p []byte) (int, error) {
	if !r.once {
		close(r.started)
		r.once = true
	}
	return r.Reader.Read(p)
}
