package productlog

import (
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const maxBytes = 256 << 10

// Only fixed event codes cross this boundary, never error strings or input data.
var allowed = map[string]bool{}

func init() {
	for _, code := range []string{"process_started", "process_stopped", "process_failed", "agent_ready", "prepare_started", "prepare_failed", "prepare_completed", "connect_started", "already_active", "not_configured", "credential_unavailable", "connection_not_configured", "peer_not_found", "peer_alias_ambiguous", "no_active_session", "disconnected", "shutdown_timeout", "safe_stop_requested", "session_failed", "session_stopped", "CONNECTED", "CONNECTING", "RECONNECTING", "DISCONNECTING", "upgrade_stop_requested"} {
		allowed[code] = true
	}
}

type record struct {
	Time  string `json:"time"`
	Code  string `json:"code"`
	Build string `json:"build"`
	PID   int    `json:"pid"`
}
type Logger struct {
	mu          sync.Mutex
	closed      bool
	queue       chan record
	done        chan struct{}
	failed      atomic.Bool
	dropped     atomic.Uint64
	dir         *securefs.Directory
	lease       io.Closer
	role, build string
}

func Open(localAppData, role string) (*Logger, error) {
	lease, err := agentinstance.AcquireLog(role)
	if err != nil {
		return nil, err
	}
	dir, err := productidentity.PrepareDiagnostics(localAppData)
	if err != nil {
		lease.Close()
		return nil, err
	}
	logger := newLogger(dir, lease, role)
	return logger, nil
}

func newLogger(dir *securefs.Directory, lease io.Closer, role string) *Logger {
	l := &Logger{dir: dir, lease: lease, role: role, build: "development", queue: make(chan record, 64), done: make(chan struct{})}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				l.build = s.Value
			}
		}
	}
	go func() {
		defer close(l.done)
		defer l.lease.Close()
		defer l.dir.Close()
		for r := range l.queue {
			if err := l.write(r); err != nil {
				l.failed.Store(true)
			}
		}
	}()
	return l
}

func (l *Logger) Event(code string) {
	if l == nil || !allowed[code] {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	r := record{time.Now().UTC().Format(time.RFC3339Nano), code, l.build, os.Getpid()}
	select {
	case l.queue <- r:
	default:
		l.dropped.Add(1)
	}
}

func (l *Logger) Failed() bool { return l != nil && (l.failed.Load() || l.dropped.Load() != 0) }

// Drain on normal shutdown; a stalled disk cannot indefinitely block app exit.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		close(l.queue)
	}
	l.mu.Unlock()
	select {
	case <-l.done:
	case <-time.After(2 * time.Second):
		l.failed.Store(true)
	}
}

func (l *Logger) write(r record) error {
	name := l.role + ".jsonl"
	f, err := l.dir.OpenReadWrite(name)
	if errors.Is(err, os.ErrNotExist) {
		f, err = l.dir.CreateExclusive(name, 0600)
	}
	if err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		f.Close()
		return err
	}
	data = append(data, '\n')
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if info.Size()+int64(len(data)) > maxBytes {
		if err = f.Close(); err != nil {
			return err
		}
		if err = l.dir.Replace(name, l.role+".previous.jsonl"); err != nil {
			return err
		}
		f, err = l.dir.CreateExclusive(name, 0600)
		if err != nil {
			return err
		}
	}
	defer f.Close()
	if _, err = f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
