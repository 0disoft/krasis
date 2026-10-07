// Package peerupdate owns explicitly delegated development-bundle delivery.
// It never sends input or applies updates. A narrow trusted callback can open
// the locally recorded UI; callers cannot supply a program or arguments.
package peerupdate

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/updatebundle"
)

var ErrDenied = errors.New("peer update not authorized")
var ErrBusy = errors.New("peer update busy")

type Status struct {
	ClipboardTransfer  ClipboardTransfer `json:"clipboardTransfer"`
	ClipboardPeer      string            `json:"clipboardPeer,omitempty"`
	Diagnostic         string            `json:"diagnostic,omitempty"`
	ReceivedDiagnostic string            `json:"receivedDiagnostic,omitempty"`
	PeerGeneration     int               `json:"peerGeneration,omitempty"`
	PeerRevision       string            `json:"peerRevision,omitempty"`
	Bytes              int64             `json:"bytes,omitempty"`
	Total              int64             `json:"total,omitempty"`
	State              string            `json:"state"`
	Allowed            []string          `json:"allowed"`
	ID                 string            `json:"id,omitempty"`
	Revision           string            `json:"revision,omitempty"`
	Generation         int               `json:"generation,omitempty"`
	Peer               string            `json:"peer,omitempty"`
}

type offer struct {
	id, hash, peer string
	data           []byte
	expires        time.Time
	created        time.Time
}
type stagedBundle interface {
	Metadata() updatebundle.Metadata
	Stage(*os.Root, string) error
}
type Manager struct {
	clipChanges       clipboardChanges
	keyboard          keyboardObservation
	appOpen           appOpenState
	OpenUI            func(string) string
	UIReady           func(time.Time) bool
	clipboard         clipboardState
	diagnostic        diagnosticTransfer
	ReceiveDiagnostic func(string, []byte) (string, error)
	ReceiveCollection func(string, string, []byte) (string, error)
	ExportDiagnostic  func(string, string) error
	peers             map[string]peerObservation
	deliveredUntil    time.Time
	workers           sync.WaitGroup
	closed            bool
	cancels           map[uint64]context.CancelFunc
	nextWorker        uint64
	Observe           func(string)
	mu                sync.Mutex
	root              *os.Root
	path              string
	generation        int
	revision          string
	allowed           map[string]bool
	status            Status
	stagedUntil       time.Time
	out               *offer
	receiving         bool
	packing           bool
	validate          func([]byte) (stagedBundle, error)
}

func Open(productRoot string, generation int) (*Manager, error) {
	if _, err := peercredentials.ValidateSecurityDirectory(productRoot); err != nil {
		return nil, err
	}
	parent, err := os.OpenRoot(productRoot)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	if err := parent.Mkdir("updates", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	path := filepath.Join(productRoot, "updates")
	if _, err := peercredentials.ProtectSecurityDirectory(path); err != nil {
		return nil, err
	}
	root, err := parent.OpenRoot("updates")
	if err != nil {
		return nil, err
	}
	m := &Manager{root: root, path: path, generation: generation, allowed: map[string]bool{}, status: Status{State: "IDLE"}, validate: func(data []byte) (stagedBundle, error) {
		return updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	}}
	m.revision = buildRevision()
	if f, e := root.Open("clipboard-peer.txt"); e == nil {
		data, e := io.ReadAll(io.LimitReader(f, 65))
		f.Close()
		if e != nil || (len(data) > 0 && !validID(string(data), 64)) {
			root.Close()
			return nil, ErrDenied
		}
		m.clipboard.peer = string(data)
	} else if !errors.Is(e, os.ErrNotExist) {
		root.Close()
		return nil, e
	}
	if generation >= 69 && m.clipboard.peer != "" {
		f, e := root.Open("clipboard-media-peer.txt")
		if e == nil {
			data, readErr := io.ReadAll(io.LimitReader(f, 65))
			_ = f.Close()
			if readErr != nil || len(data) > 64 || string(data) != m.clipboard.peer {
				m.clipboard.peer = "" // Old text-only consent cannot grant image sharing.
			} else {
				m.clipboard.imageAllowed = true
			}
		} else if errors.Is(e, os.ErrNotExist) {
			m.clipboard.peer = ""
		} else {
			root.Close()
			return nil, e
		}
	}
	if generation >= 70 && m.clipboard.peer != "" {
		f, e := root.Open("clipboard-files-peer.txt")
		if e == nil {
			data, readErr := io.ReadAll(io.LimitReader(f, 65))
			_ = f.Close()
			if readErr != nil || len(data) > 64 || string(data) != m.clipboard.peer {
				m.clipboard.peer = "" // Earlier consent did not include files.
			} else {
				m.clipboard.filesAllowed = true
			}
		} else if errors.Is(e, os.ErrNotExist) {
			m.clipboard.peer = ""
		} else {
			root.Close()
			return nil, e
		}
	}
	f, err := root.Open("permissions.json")
	if err == nil {
		defer f.Close()
		data, readErr := io.ReadAll(io.LimitReader(f, 8193))
		if readErr != nil || len(data) > 8192 || json.Unmarshal(data, &m.allowed) != nil {
			root.Close()
			return nil, ErrDenied
		}
		for fp := range m.allowed {
			if !validID(fp, 64) {
				root.Close()
				return nil, ErrDenied
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		root.Close()
		return nil, err
	}
	return m, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	for _, cancel := range m.cancels {
		cancel()
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.workers.Wait(); close(done) }()
	select {
	case <-done:
		return m.root.Close()
	case <-time.After(5 * time.Second):
		return ErrBusy
	}
}

func (m *Manager) Start(ctx context.Context, address, peer string, listener bool, config *tls.Config, authorized func() bool) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	run, cancel := context.WithCancel(ctx)
	if m.cancels == nil {
		m.cancels = map[uint64]context.CancelFunc{}
	}
	m.nextWorker++
	id := m.nextWorker
	m.cancels[id] = cancel
	m.workers.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.workers.Done()
		defer cancel()
		defer func() { m.mu.Lock(); delete(m.cancels, id); m.mu.Unlock() }()
		for run.Err() == nil {
			m.Run(run, address, peer, listener, config, authorized)
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-run.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.out != nil && m.status.State == "QUEUED" && !m.out.created.IsZero() && time.Since(m.out.created) > 15*time.Second && m.peers[m.out.peer].at.Before(m.out.created) {
		m.out = nil
		m.status.State = "PEER_UNREACHABLE"
	}
	if m.status.State == "DELIVERED" && !m.deliveredUntil.IsZero() && time.Now().After(m.deliveredUntil) {
		m.status.State = "RESTART_UNCONFIRMED"
	}
	if m.status.State == "STAGED" && !m.stagedUntil.IsZero() && time.Now().After(m.stagedUntil) {
		m.status.State = "RECEIVE_FAILED"
	}
	if m.status.State == "READY" && (m.stagedUntil.IsZero() || time.Now().After(m.stagedUntil)) {
		m.status.State = "EXPIRED"
	}
	if m.out != nil && time.Now().After(m.out.expires) {
		m.out = nil
		m.status.State = "EXPIRED"
	}
	s := m.status
	s.ClipboardPeer = m.ClipboardPeer()
	s.ClipboardTransfer = m.ClipboardTransferStatus()
	m.diagnostic.mu.Lock()
	if m.diagnostic.out != nil && time.Now().After(m.diagnostic.out.expires) {
		m.diagnostic.out = nil
		m.diagnostic.state = "EXPIRED"
	}
	s.Diagnostic = m.diagnostic.state
	s.ReceivedDiagnostic = m.diagnostic.received
	m.diagnostic.mu.Unlock()
	s.Allowed = []string{}
	for fp, allow := range m.allowed {
		if allow {
			s.Allowed = append(s.Allowed, fp)
		}
	}
	return s
}
func (m *Manager) Permit(peer string, allowed bool) error {
	if !validID(peer, 64) {
		return ErrDenied
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State == "APPLYING" {
		return ErrBusy
	}
	next := map[string]bool{}
	for fp, v := range m.allowed {
		if v {
			next[fp] = true
		}
	}
	if allowed {
		next[peer] = true
	} else {
		delete(next, peer)
	}
	data, err := json.Marshal(next)
	if err != nil || len(data) > 8192 {
		return ErrDenied
	}
	name := randomID() + ".tmp"
	f, err := m.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer m.root.Remove(name)
	_, err = f.Write(data)
	err = errors.Join(err, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	if err = m.root.Rename(name, "permissions.json"); err != nil {
		return err
	}
	m.allowed = next
	m.event("peer_update_permission_saved")
	if !allowed && m.status.Peer == peer && (m.status.State == "READY" || m.status.State == "STAGED") {
		m.status.State = "REVOKED"
	}
	return nil
}
func (m *Manager) permitted(peer string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allowed[peer]
}

// Prepare is local-only and snapshots before queueing. No network caller chooses
// a source path. A failed or expired offer needs another explicit user click.
func (m *Manager) Prepare(peer, directory string) error {
	m.mu.Lock()
	if !validID(peer, 64) || m.packing || m.receiving || m.out != nil || m.status.State == "READY" || m.status.State == "STAGED" || m.status.State == "APPLYING" {
		m.mu.Unlock()
		return ErrBusy
	}
	m.packing = true
	m.status = Status{State: "PREPARING", Peer: peer}
	m.mu.Unlock()
	root, err := os.OpenRoot(directory)
	var data []byte
	if err == nil {
		data, err = updatebundle.Pack(root)
		root.Close()
	}
	var metadata updatebundle.Metadata
	if err == nil {
		var bundle *updatebundle.Bundle
		bundle, err = updatebundle.Read(bytes.NewReader(data), int64(len(data)))
		if err == nil {
			metadata = bundle.Metadata()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.packing = false
	if err != nil {
		m.status.State = FailureCode(err)
		return err
	}
	id := randomID()
	hash := sha256.Sum256(data)
	m.out = &offer{id: id, hash: hex.EncodeToString(hash[:]), peer: peer, data: data, created: time.Now(), expires: time.Now().Add(3 * time.Minute)}
	m.status = Status{State: "QUEUED", Peer: peer, ID: id, Generation: metadata.Generation, Revision: metadata.Revision, Total: int64(len(data))}
	m.event("peer_update_queued")
	return nil
}
func (m *Manager) offered(peer string) *offer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.out == nil || m.out.peer != peer {
		return nil
	}
	if time.Now().After(m.out.expires) {
		m.out = nil
		m.status.State = "EXPIRED"
		return nil
	}
	copy := *m.out
	return &copy
}
func (m *Manager) sent(id, state string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.out != nil && m.out.id == id {
		m.out = nil
		m.status.State = state
		if state == "DELIVERED" {
			m.deliveredUntil = time.Now().Add(3 * time.Minute)
		}
	}
}

func (m *Manager) receive(peer, id, expected string, reader io.Reader, authorized func() bool) error {
	if !validID(id, 32) || !validID(expected, 64) || !authorized() {
		return ErrDenied
	}
	m.mu.Lock()
	if !m.allowed[peer] {
		m.mu.Unlock()
		return ErrDenied
	}
	if m.status.ID == id && m.status.Peer == peer && (m.status.State == "READY" || m.status.State == "STAGED") {
		m.mu.Unlock()
		return nil
	}
	if m.receiving || m.packing || m.out != nil || m.status.State == "READY" || m.status.State == "STAGED" || m.status.State == "APPLYING" {
		m.mu.Unlock()
		return ErrBusy
	}
	m.receiving = true
	m.status = Status{State: "RECEIVING", ID: id, Peer: peer}
	m.mu.Unlock()
	var received stagedBundle
	var failure error
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.receiving = false
		if failure != nil {
			m.status.State = FailureCode(failure)
			m.event("peer_update_receive_failed")
		}
	}()
	progress := &progressReader{Reader: reader, update: func(n int64) { m.progress(peer, id, "RECEIVING", n, 0) }}
	data, err := io.ReadAll(io.LimitReader(progress, updatebundle.MaxArchiveBytes+1))
	if err != nil {
		failure = ErrTransfer
		return failure
	}
	if int64(len(data)) > updatebundle.MaxArchiveBytes {
		failure = updatebundle.ErrInvalid
		return failure
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != expected {
		failure = ErrHashMismatch
		return failure
	}
	m.progress(peer, id, "VERIFYING", int64(len(data)), int64(len(data)))
	received, err = m.validate(data)
	if err != nil {
		failure = err
		return err
	}
	if err := compareVersion(m.generation, m.revision, received.Metadata().Generation, received.Metadata().Revision); err != nil {
		failure = err
		return failure
	}
	if !authorized() {
		failure = ErrDenied
		return failure
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.allowed[peer] {
		failure = ErrDenied
		return failure
	}
	if err = received.Stage(m.root, id); err != nil {
		failure = errors.Join(ErrStorage, err)
		return failure
	}
	m.status = Status{State: "STAGED", ID: id, Peer: peer, Revision: received.Metadata().Revision, Generation: received.Metadata().Generation}
	m.stagedUntil = time.Now().Add(3 * time.Minute)
	return nil
}

// Claim consumes a verified ready job under the same lock as permission changes.
// The caller must revalidate paired trust and stop the input session before run.
func (m *Manager) Claim(id, peer string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State == "READY" && (m.stagedUntil.IsZero() || time.Now().After(m.stagedUntil)) {
		m.status.State = "EXPIRED"
		return "", "", ErrDenied
	}
	if m.status.State != "READY" || m.status.ID != id || m.status.Peer != peer || !m.allowed[peer] {
		return "", "", ErrDenied
	}
	m.status.State = "APPLYING"
	m.event("peer_update_apply_started")
	return filepath.Join(m.path, id), m.status.Revision, nil
}
func (m *Manager) ApplyFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State == "APPLYING" {
		m.status.State = "APPLY_FAILED"
		m.event("peer_update_apply_failed")
	}
}
func (m *Manager) event(code string) {
	if m.Observe != nil {
		m.Observe(code)
	}
}
func (m *Manager) ready(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State == "STAGED" && !m.stagedUntil.IsZero() && time.Now().After(m.stagedUntil) {
		m.status.State = "RECEIVE_FAILED"
	}
	if m.status.ID == id && m.status.State == "STAGED" && m.allowed[m.status.Peer] {
		m.status.State = "READY"
		m.event("peer_update_ready")
	}
}
func randomID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}
func validID(s string, n int) bool {
	if len(s) != n {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && hex.EncodeToString(b) == s
}
