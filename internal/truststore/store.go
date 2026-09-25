package truststore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/transport"
)

const (
	storeFilename = "trust.json"
	lockFilename  = ".trust.lock"
	maxStoreBytes = 1 << 20
)

var (
	ErrStoreNotFound = errors.New("trust store does not exist")
	ErrPeerExists    = errors.New("trusted peer already exists")
	ErrPeerNotFound  = errors.New("trusted peer does not exist")
	ErrPeerRevoked   = errors.New("trusted peer is already revoked")
	ErrStoreBusy     = errors.New("trust store is busy")
)

type Store struct {
	directory string
}

// ReadTrace records only time spent in fixed trust-read phases. It is safe to
// snapshot while a bounded monitor is waiting for a blocked filesystem call.
type ReadTrace struct {
	mu        sync.Mutex
	started   time.Time
	entered   time.Time
	phase     string
	durations map[string]time.Duration
	attempts  int
}

type ReadTraceSnapshot struct {
	Phase     string
	Durations map[string]time.Duration
	Attempts  int
	Elapsed   time.Duration
}

func NewReadTrace() *ReadTrace {
	return &ReadTrace{started: time.Now(), durations: make(map[string]time.Duration, 5)}
}

func (trace *ReadTrace) enter(phase string) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	now := time.Now()
	if trace.phase != "" {
		trace.durations[trace.phase] += now.Sub(trace.entered)
	}
	trace.phase, trace.entered = phase, now
	if phase == "prelock" {
		trace.attempts++
	}
}

func (trace *ReadTrace) finish() {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if trace.phase != "" {
		trace.durations[trace.phase] += time.Since(trace.entered)
		trace.phase = ""
	}
}

func (trace *ReadTrace) EnterRetryWait() { trace.enter("retry_wait") }

func (trace *ReadTrace) Snapshot() ReadTraceSnapshot {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	now := time.Now()
	snapshot := ReadTraceSnapshot{Phase: trace.phase, Durations: make(map[string]time.Duration, len(trace.durations)+1), Attempts: trace.attempts, Elapsed: now.Sub(trace.started)}
	for phase, duration := range trace.durations {
		snapshot.Durations[phase] = duration
	}
	if trace.phase != "" {
		snapshot.Durations[trace.phase] += now.Sub(trace.entered)
	}
	return snapshot
}

func New(directory string) (*Store, error) {
	if directory == "" {
		return nil, errors.New("trust store directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve trust store directory: %w", err)
	}
	return &Store{directory: absolute}, nil
}

func (store *Store) Import(peer transport.AuthenticatedPeerContext, alias string, allowInboundControl bool, now time.Time) (PeerRecord, error) {
	return store.importPeer(peer, alias, allowInboundControl, now, nil)
}

func (store *Store) importPeer(peer transport.AuthenticatedPeerContext, alias string, allowInboundControl bool, now time.Time, prepare func(*securefs.Directory) error) (PeerRecord, error) {
	if store == nil {
		return PeerRecord{}, errors.New("trust store is not initialized")
	}
	if err := peer.Validate(); err != nil {
		return PeerRecord{}, err
	}
	if err := validateAlias(alias); err != nil {
		return PeerRecord{}, err
	}
	if now.IsZero() {
		return PeerRecord{}, errors.New("import time is required")
	}
	record := PeerRecord{
		Fingerprint: peer.CertificateSHA256, Role: peer.Role, Alias: alias,
		AllowInboundControl: allowInboundControl, CreatedAt: now.UTC(),
	}
	err := store.mutatePrepared(true, now.UTC(), func(document *Document) error {
		for _, existing := range document.Peers {
			if existing.Fingerprint == record.Fingerprint {
				return ErrPeerExists
			}
		}
		document.Peers = append(document.Peers, record)
		return nil
	}, prepare)
	if err != nil {
		return PeerRecord{}, err
	}
	return record, nil
}

func (store *Store) Revoke(fingerprint string, now time.Time) (PeerRecord, error) {
	if store == nil {
		return PeerRecord{}, errors.New("trust store is not initialized")
	}
	if err := validateFingerprint(fingerprint); err != nil {
		return PeerRecord{}, err
	}
	if now.IsZero() {
		return PeerRecord{}, errors.New("revocation time is required")
	}
	var revoked PeerRecord
	err := store.mutate(false, now.UTC(), func(document *Document) error {
		for index := range document.Peers {
			if document.Peers[index].Fingerprint != fingerprint {
				continue
			}
			if document.Peers[index].RevokedAt != nil {
				return ErrPeerRevoked
			}
			revokedAt := now.UTC()
			document.Peers[index].RevokedAt = &revokedAt
			revoked = document.Peers[index]
			return nil
		}
		return ErrPeerNotFound
	})
	return revoked, err
}

func (store *Store) Update(fingerprint, alias string, allowInboundControl bool, now time.Time) (PeerRecord, error) {
	if store == nil {
		return PeerRecord{}, errors.New("trust store is not initialized")
	}
	if err := validateFingerprint(fingerprint); err != nil {
		return PeerRecord{}, err
	}
	if err := validateAlias(alias); err != nil {
		return PeerRecord{}, err
	}
	if now.IsZero() {
		return PeerRecord{}, errors.New("update time is required")
	}
	var updated PeerRecord
	err := store.mutate(false, now.UTC(), func(document *Document) error {
		for index := range document.Peers {
			if document.Peers[index].Fingerprint != fingerprint {
				continue
			}
			if document.Peers[index].RevokedAt != nil {
				return ErrPeerRevoked
			}
			document.Peers[index].Alias = alias
			document.Peers[index].AllowInboundControl = allowInboundControl
			updated = document.Peers[index]
			return nil
		}
		return ErrPeerNotFound
	})
	return updated, err
}

func (store *Store) Replace(fingerprint string, replacement transport.AuthenticatedPeerContext, alias string, allowInboundControl bool, now time.Time) (PeerRecord, PeerRecord, error) {
	return store.replacePeer(fingerprint, replacement, alias, allowInboundControl, now, nil)
}

func (store *Store) replacePeer(fingerprint string, replacement transport.AuthenticatedPeerContext, alias string, allowInboundControl bool, now time.Time, prepare func(*securefs.Directory) error) (PeerRecord, PeerRecord, error) {
	if store == nil {
		return PeerRecord{}, PeerRecord{}, errors.New("trust store is not initialized")
	}
	if err := validateFingerprint(fingerprint); err != nil {
		return PeerRecord{}, PeerRecord{}, err
	}
	if err := replacement.Validate(); err != nil {
		return PeerRecord{}, PeerRecord{}, err
	}
	if replacement.CertificateSHA256 == fingerprint {
		return PeerRecord{}, PeerRecord{}, errors.New("replacement fingerprint must differ from the existing peer")
	}
	if err := validateAlias(alias); err != nil {
		return PeerRecord{}, PeerRecord{}, err
	}
	if now.IsZero() {
		return PeerRecord{}, PeerRecord{}, errors.New("replacement time is required")
	}
	var revoked, added PeerRecord
	err := store.mutatePrepared(false, now.UTC(), func(document *Document) error {
		oldIndex := -1
		for index := range document.Peers {
			record := document.Peers[index]
			if record.Fingerprint == replacement.CertificateSHA256 {
				return ErrPeerExists
			}
			if record.Fingerprint == fingerprint {
				oldIndex = index
			}
		}
		if oldIndex < 0 {
			return ErrPeerNotFound
		}
		if document.Peers[oldIndex].RevokedAt != nil {
			return ErrPeerRevoked
		}
		revokedAt := now.UTC()
		document.Peers[oldIndex].RevokedAt = &revokedAt
		revoked = document.Peers[oldIndex]
		added = PeerRecord{
			Fingerprint:         replacement.CertificateSHA256,
			Role:                replacement.Role,
			Alias:               alias,
			AllowInboundControl: allowInboundControl,
			CreatedAt:           now.UTC(),
		}
		document.Peers = append(document.Peers, added)
		return nil
	}, prepare)
	if err != nil {
		return PeerRecord{}, PeerRecord{}, err
	}
	return revoked, added, err
}

func (store *Store) List() (Document, error) {
	if store == nil {
		return Document{}, errors.New("trust store is not initialized")
	}
	return store.readLocked()
}

func (store *Store) mutate(create bool, now time.Time, change func(*Document) error) error {
	return store.mutatePrepared(create, now, change, nil)
}

func (store *Store) mutatePrepared(create bool, now time.Time, change func(*Document) error, prepare func(*securefs.Directory) error) error {
	document, directory, release, err := store.openLocked(create)
	if err != nil {
		return err
	}
	defer release()
	if err := change(&document); err != nil {
		return err
	}
	document.SchemaVersion = SchemaVersion
	document.Revision++
	document.UpdatedAt = now
	if document.Peers == nil {
		document.Peers = []PeerRecord{}
	}
	sortPeers(document.Peers)
	if err := document.Validate(); err != nil {
		return err
	}
	if prepare != nil {
		if err := prepare(directory); err != nil {
			return err
		}
	}
	return store.write(directory, document)
}

func (store *Store) readLocked() (Document, error) {
	document, _, release, err := store.openReadLocked()
	if err != nil {
		return Document{}, err
	}
	defer release()
	return document, nil
}

func (store *Store) openReadLocked() (Document, *securefs.Directory, func(), error) {
	return store.openLockedMode(false, true, nil)
}

func (store *Store) openReadLockedTraced(trace *ReadTrace) (Document, *securefs.Directory, func(), error) {
	return store.openLockedMode(false, true, trace)
}

func (store *Store) openLocked(create bool) (Document, *securefs.Directory, func(), error) {
	return store.openLockedMode(create, false, nil)
}

func (store *Store) openLockedMode(create, shared bool, trace *ReadTrace) (Document, *securefs.Directory, func(), error) {
	trace.enter("prelock")
	if err := store.prepareDirectory(create); err != nil {
		return Document{}, nil, nil, err
	}
	directory, err := securefs.OpenDirectory(store.directory)
	if err != nil {
		return Document{}, nil, nil, fmt.Errorf("open trust store directory handle: %w", err)
	}
	lockFile, created, err := openLockFile(directory)
	if err != nil {
		_ = directory.Close()
		return Document{}, nil, nil, err
	}
	closeLock := func() {
		_ = lockFile.Close()
		_ = directory.Close()
	}
	if created {
		if err := peercredentials.ProtectSecurityFileHandle(lockFile); err != nil {
			closeLock()
			return Document{}, nil, nil, err
		}
	} else if err := peercredentials.ValidateSecurityFileHandle(lockFile); err != nil {
		closeLock()
		return Document{}, nil, nil, err
	}
	trace.enter("lock")
	unlock, err := acquireFileLock(lockFile, shared)
	if err != nil {
		closeLock()
		return Document{}, nil, nil, err
	}
	release := func() {
		_ = unlock()
		_ = lockFile.Close()
		_ = directory.Close()
	}
	trace.enter("store_read")
	document, err := store.read(directory)
	if err != nil {
		if create && errors.Is(err, ErrStoreNotFound) {
			return Document{SchemaVersion: SchemaVersion, Peers: []PeerRecord{}}, directory, release, nil
		}
		release()
		return Document{}, nil, nil, err
	}
	return document, directory, release, nil
}

func (store *Store) prepareDirectory(create bool) error {
	if _, err := peercredentials.ValidateSecurityDirectory(filepath.Dir(store.directory)); err != nil {
		return fmt.Errorf("validate trust store parent security: %w", err)
	}
	info, err := os.Lstat(store.directory)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return ErrStoreNotFound
		}
		if err := os.Mkdir(store.directory, 0o700); err != nil {
			return fmt.Errorf("create trust store directory: %w", err)
		}
		if _, err = peercredentials.ProtectSecurityDirectory(store.directory); err != nil {
			_ = os.Remove(store.directory)
			return err
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect trust store directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("trust store path must be a directory")
	}
	_, err = peercredentials.ValidateSecurityDirectory(store.directory)
	return err
}

func (store *Store) read(directory *securefs.Directory) (Document, error) {
	file, err := directory.Open(storeFilename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Document{}, ErrStoreNotFound
		}
		return Document{}, fmt.Errorf("open trust store: %w", err)
	}
	defer file.Close()
	if err := peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return Document{}, fmt.Errorf("validate trust store security: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStoreBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("read trust store: %w", err)
	}
	if len(data) > maxStoreBytes {
		return Document{}, errors.New("trust store exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode trust store: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Document{}, err
	}
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (store *Store) write(directory *securefs.Directory, document Document) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trust store: %w", err)
	}
	data = append(data, '\n')
	temporary, temporaryName, err := directory.CreateTemporary(".trust-", 0o600)
	if err != nil {
		return fmt.Errorf("create trust store temporary file: %w", err)
	}
	defer directory.Remove(temporaryName)
	if err := peercredentials.ProtectSecurityFileHandle(temporary); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write trust store temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("flush trust store temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := directory.Replace(temporaryName, storeFilename); err != nil {
		return fmt.Errorf("replace trust store: %w", err)
	}
	file, err := directory.Open(storeFilename)
	if err != nil {
		return err
	}
	defer file.Close()
	return peercredentials.ValidateSecurityFileHandle(file)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trust store contains trailing JSON")
		}
		return fmt.Errorf("decode trailing trust store data: %w", err)
	}
	return nil
}

func openLockFile(directory *securefs.Directory) (*os.File, bool, error) {
	file, err := directory.CreateExclusive(lockFilename, 0o600)
	if err == nil {
		return file, true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, false, fmt.Errorf("create trust store lock: %w", err)
	}
	file, err = directory.OpenReadWrite(lockFilename)
	if err != nil {
		return nil, false, fmt.Errorf("open trust store lock: %w", err)
	}
	return file, false, nil
}
