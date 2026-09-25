package productlog

import (
	"crypto/rand"
	"encoding/hex"
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
	for _, code := range []string{"suspend", "resume", "session_lock", "session_disconnect", "logoff", "shutdown", "display_change", "desktop_switch", "monitor_failed"} {
		allowed["lifecycle_"+code] = true
	}
	for _, code := range []string{"tcp_keepalive_15_5_3", "tcp_keepalive_unavailable", "tcp_keepalive_not_tcp"} {
		allowed[code] = true
	}
	for _, code := range []string{"session_trust_read_timeout", "session_trust_read_pending", "session_trust_read_deferred", "session_trust_read_recovered", "session_trust_freshness_expired", "session_trust_revoked", "session_trust_peer_missing", "session_trust_store_missing"} {
		allowed[code] = true
	}
	for _, code := range []string{"session_certificate_deadline", "session_guard_exited", "session_guard_close_failed", "session_trust_check_failed", "session_trust_busy_exhausted"} {
		allowed[code] = true
	}
	for _, phase := range []string{
		"prelock", "prelock_parent_acl", "prelock_directory_lstat", "prelock_directory_acl",
		"prelock_directory_open", "prelock_lockfile_open", "prelock_lockfile_acl",
		"lock", "store_read", "store_read_open", "store_read_acl", "store_read_bytes",
		"store_read_decode", "store_read_close", "certificate", "retry_wait",
	} {
		allowed["trust_active_"+phase] = true
		for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			allowed["trust_timing_"+phase+"_"+bucket] = true
		}
	}
	allowed["boundary_summary"] = true
	allowed["protected_input_detected"] = true
	allowed["peer_disconnect_requested"] = true
	allowed["disconnect_notification_failed"] = true
	for _, code := range []string{"process_started", "process_stopped", "process_failed", "agent_ready", "prepare_started", "prepare_failed", "prepare_completed", "connect_started", "already_active", "not_configured", "credential_unavailable", "connection_not_configured", "peer_not_found", "peer_alias_ambiguous", "no_active_session", "disconnected", "shutdown_timeout", "safe_stop_requested", "session_failed", "session_stopped", "CONNECTED", "CONNECTING", "RECONNECTING", "DISCONNECTING", "upgrade_stop_requested"} {
		allowed[code] = true
	}
}

type record struct {
	Time          string            `json:"time"`
	Code          string            `json:"code"`
	Build         string            `json:"build"`
	BuildDirty    bool              `json:"buildDirty,omitempty"`
	Run           string            `json:"run,omitempty"`
	Connection    string            `json:"connection,omitempty"`
	Sequence      uint64            `json:"sequence,omitempty"`
	ElapsedMicros int64             `json:"elapsedMicros,omitempty"`
	PID           int               `json:"pid"`
	Boundary      *BoundarySummary  `json:"boundary,omitempty"`
	Injection     *InjectionFailure `json:"injection,omitempty"`
	Retry         *RetrySummary     `json:"retry,omitempty"`
	Runtime       *RuntimeSummary   `json:"runtime,omitempty"`
}
type Logger struct {
	mu              sync.Mutex
	closed          bool
	queue           chan record
	done            chan struct{}
	failed          atomic.Bool
	dropped         atomic.Uint64
	dir             *securefs.Directory
	lease           io.Closer
	role, build     string
	buildDirty      bool
	run, connection string
	sequence        uint64
	started         time.Time
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
	l.started = time.Now()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err == nil {
		l.run = hex.EncodeToString(nonce[:])
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				l.build = s.Value
			}
			if s.Key == "vcs.modified" {
				l.buildDirty = s.Value == "true"
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
	l.enqueue(code, nil)
}

func (l *Logger) enqueue(code string, boundary *BoundarySummary) {
	l.enqueueRecord(record{Code: code, Boundary: boundary})
}

func (l *Logger) enqueueRecord(r record) {
	code := r.Code
	if l == nil || !allowed[code] {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	r.Time = time.Now().UTC().Format(time.RFC3339Nano)
	r.Build = l.build
	r.PID = os.Getpid()
	l.sequence++
	r.BuildDirty = l.buildDirty
	r.Run = l.run
	r.Connection = l.connection
	r.Sequence = l.sequence
	if !l.started.IsZero() {
		r.ElapsedMicros = time.Since(l.started).Microseconds()
	}
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
