package truststore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
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
	err := store.mutate(true, now.UTC(), func(document *Document) error {
		for _, existing := range document.Peers {
			if existing.Fingerprint == record.Fingerprint {
				return ErrPeerExists
			}
		}
		document.Peers = append(document.Peers, record)
		return nil
	})
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

func (store *Store) List() (Document, error) {
	if store == nil {
		return Document{}, errors.New("trust store is not initialized")
	}
	return store.readLocked(false)
}

func (store *Store) mutate(create bool, now time.Time, change func(*Document) error) error {
	document, release, err := store.openLocked(create)
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
	return store.write(document)
}

func (store *Store) readLocked(create bool) (Document, error) {
	document, release, err := store.openLocked(create)
	if err != nil {
		return Document{}, err
	}
	defer release()
	return document, nil
}

func (store *Store) openLocked(create bool) (Document, func(), error) {
	if err := store.prepareDirectory(create); err != nil {
		return Document{}, nil, err
	}
	lockPath := filepath.Join(store.directory, lockFilename)
	lockFile, created, err := openLockFile(lockPath)
	if err != nil {
		return Document{}, nil, err
	}
	closeLock := func() { _ = lockFile.Close() }
	if created {
		if err := peercredentials.ProtectSecurityFile(lockPath); err != nil {
			closeLock()
			return Document{}, nil, err
		}
	} else if err := peercredentials.ValidateSecurityFile(lockPath); err != nil {
		closeLock()
		return Document{}, nil, err
	}
	unlock, err := acquireFileLock(lockFile)
	if err != nil {
		closeLock()
		return Document{}, nil, err
	}
	release := func() {
		_ = unlock()
		_ = lockFile.Close()
	}
	document, err := store.read()
	if err != nil {
		if create && errors.Is(err, ErrStoreNotFound) {
			return Document{SchemaVersion: SchemaVersion, Peers: []PeerRecord{}}, release, nil
		}
		release()
		return Document{}, nil, err
	}
	return document, release, nil
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

func (store *Store) read() (Document, error) {
	path := filepath.Join(store.directory, storeFilename)
	if err := peercredentials.ValidateSecurityFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Document{}, ErrStoreNotFound
		}
		return Document{}, fmt.Errorf("validate trust store security: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return Document{}, fmt.Errorf("open trust store: %w", err)
	}
	defer file.Close()
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

func (store *Store) write(document Document) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trust store: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(store.directory, ".trust-*.tmp")
	if err != nil {
		return fmt.Errorf("create trust store temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := peercredentials.ProtectSecurityFile(temporaryPath); err != nil {
		return err
	}
	temporary, err = os.OpenFile(temporaryPath, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
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
	if err := replaceFile(temporaryPath, filepath.Join(store.directory, storeFilename)); err != nil {
		return fmt.Errorf("replace trust store: %w", err)
	}
	return peercredentials.ValidateSecurityFile(filepath.Join(store.directory, storeFilename))
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

func openLockFile(path string) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err == nil {
		return file, true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, false, fmt.Errorf("create trust store lock: %w", err)
	}
	file, err = os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open trust store lock: %w", err)
	}
	return file, false, nil
}
