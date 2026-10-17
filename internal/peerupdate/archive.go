package peerupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/updatebundle"
)

func archiveNameFor(id string) string { return "archive-" + id + ".zip" }

func (m *Manager) closeArchive(file *os.File, name string) error {
	var err error
	if file != nil {
		err = file.Close()
	}
	if name != "" {
		err = errors.Join(err, m.root.Remove(name))
	}
	if err != nil {
		m.event("peer_stage_cleanup_failed")
		return errors.Join(ErrStorage, err)
	}
	return nil
}

func (m *Manager) prepareArchive(directory string) (result *offer, metadata updatebundle.Metadata, err error) {
	id := randomID()
	name := archiveNameFor(id)
	file, err := m.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, metadata, errors.Join(ErrStorage, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, m.closeArchive(file, name))
		}
	}()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, metadata, err
	}
	hash := sha256.New()
	storage := &archiveStorageWriter{Writer: file}
	err = errors.Join(updatebundle.PackTo(root, io.MultiWriter(storage, hash)), root.Close())
	if storage.err != nil {
		err = errors.Join(ErrStorage, err)
	}
	if err != nil {
		return nil, metadata, err
	}
	if err = file.Sync(); err != nil {
		return nil, metadata, errors.Join(ErrStorage, err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, metadata, err
	}
	// Validation also streams expanded files to a short-lived private directory.
	verified, err := updatebundle.ReadStaged(file, info.Size(), m.root, randomID())
	if err != nil {
		return nil, metadata, err
	}
	metadata = verified.Metadata()
	if err = verified.Close(); err != nil {
		return nil, metadata, errors.Join(ErrStorage, err)
	}
	return &offer{id: id, hash: hex.EncodeToString(hash.Sum(nil)), file: file, name: name, size: info.Size()}, metadata, nil
}

// Each transfer gets an independent offset, while the offer owns the file.
// Retirement removes the queue entry immediately; the final reader deletes it.
type offerLease struct {
	*offer
	reader  *io.SectionReader
	once    sync.Once
	release func()
}

func (lease *offerLease) Close() { lease.once.Do(lease.release) }

type offerRequestBody struct {
	io.Reader
	lease *offerLease
}

// net/http may close an upload body asynchronously after Do returns. Keep its
// reader lease until that close, rather than retiring its file with the response.
func (body *offerRequestBody) Close() error { body.lease.Close(); return nil }

func (m *Manager) armOfferExpiryLocked(archive *offer) {
	archive.timer = time.AfterFunc(time.Until(archive.expires), func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.out == archive {
			m.retireOfferLocked("EXPIRED")
		}
	})
}

func (m *Manager) offered(peer string) *offerLease {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.out == nil || m.out.peer != peer {
		return nil
	}
	if time.Now().After(m.out.expires) {
		m.retireOfferLocked("EXPIRED")
		return nil
	}
	current := m.out
	if current.file == nil {
		return nil
	}
	current.readers++
	m.workers.Add(1)
	return &offerLease{offer: current, reader: io.NewSectionReader(current.file, 0, current.size), release: func() {
		m.mu.Lock()
		current.readers--
		if current.retired && current.readers == 0 {
			if err := m.closeArchive(current.file, current.name); err != nil && m.status.ID == current.id {
				m.status.State = "STORAGE_FAILED"
			}
		}
		m.mu.Unlock()
		m.workers.Done()
	}}
}

// Caller holds mu. Also used for denied/expired offers and shutdown, not just ACKs.
func (m *Manager) retireOfferLocked(state string) error {
	current := m.out
	if current == nil {
		return nil
	}
	m.out = nil
	current.retired = true
	if current.timer != nil {
		current.timer.Stop()
	}
	m.status.State = state
	if current.readers == 0 {
		if err := m.closeArchive(current.file, current.name); err != nil {
			m.status.State = "STORAGE_FAILED"
			return err
		}
	}
	return nil
}

type archiveStorageWriter struct {
	io.Writer
	err error
}

func (writer *archiveStorageWriter) Write(data []byte) (int, error) {
	n, err := writer.Writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		writer.err = err
	}
	return n, err
}
