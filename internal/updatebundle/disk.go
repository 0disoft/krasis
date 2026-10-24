package updatebundle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/0disoft/krasis/internal/ziplimit"
)

type diskDigest struct {
	size int64
	hash string
}

// DiskBundle owns a private staging directory, not executable byte slices.
// Close discards it unless Stage has transferred ownership to the receiver.
type DiskBundle struct {
	parent    *os.Root
	directory string
	metadata  Metadata
	files     map[string]diskDigest
}

func (b *DiskBundle) Metadata() Metadata { return b.metadata }

func (b *DiskBundle) Close() error {
	if b == nil || b.directory == "" {
		return nil
	}
	err := b.parent.RemoveAll(b.directory)
	if err == nil {
		b.directory = ""
	}
	return err
}

// ReadStaged extracts only fixed, allowlisted names into a fresh directory.
// Digests, metadata, PE headers and Go build provenance use the same validation
// rules as Read. Executables are inspected through ReaderAt without ReadAll.
func ReadStaged(source io.ReaderAt, size int64, parent *os.Root, directory string) (*DiskBundle, error) {
	return readStaged(source, size, parent, directory, verifyExecutableReader)
}

func readStaged(source io.ReaderAt, size int64, parent *os.Root, directory string, executable func(string, io.ReaderAt, string) error) (bundle *DiskBundle, err error) {
	if source == nil || size <= 0 || size > MaxArchiveBytes || parent == nil || !validHex(directory, 32) {
		return nil, ErrInvalid
	}
	archive, err := ziplimit.NewReader(source, size, len(installerNames)+len(probeNames)+len(uiAccessProbeNames))
	if err != nil || len(archive.File) < len(legacyNames) || len(archive.File) > len(installerNames)+len(probeNames)+len(uiAccessProbeNames) || !archiveWithinExpandedLimit(archive.File) {
		return nil, ErrInvalid
	}
	if err = parent.Mkdir(directory, 0700); err != nil {
		return nil, err
	}
	root, err := parent.OpenRoot(directory)
	if err != nil {
		return nil, errors.Join(err, parent.RemoveAll(directory))
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			err = errors.Join(err, parent.RemoveAll(directory))
			bundle = nil
		}
	}()
	index := map[string][]byte{}
	digests := map[string]diskDigest{}
	var total int64
	for _, entry := range archive.File {
		limit := fileLimit(entry.Name)
		if limit == 0 || entry.Mode()&os.ModeType != 0 || entry.Flags&1 != 0 || entry.UncompressedSize64 > uint64(limit) || index[entry.Name] != nil {
			return nil, ErrInvalid
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, errors.Join(ErrInvalid, err)
		}
		file, err := root.OpenFile(entry.Name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, errors.Join(err, stream.Close())
		}
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(stream, min(limit, MaxExpandedBytes-total)+1))
		copyErr = errors.Join(copyErr, stream.Close(), file.Sync(), file.Close())
		total += n
		if copyErr != nil || n > limit || total > MaxExpandedBytes || uint64(n) != entry.UncompressedSize64 {
			return nil, errors.Join(ErrInvalid, copyErr)
		}
		index[entry.Name] = []byte{} // Presence marker; executable data stays on disk.
		digests[entry.Name] = diskDigest{size: n, hash: hex.EncodeToString(hash.Sum(nil))}
		if entry.Name == "BUILD.json" || entry.Name == "SHA256SUMS.txt" {
			metadata, openErr := root.Open(entry.Name)
			if openErr != nil {
				return nil, openErr
			}
			index[entry.Name], err = io.ReadAll(io.LimitReader(metadata, 8193))
			err = errors.Join(err, metadata.Close())
			if err != nil {
				return nil, err
			}
			got := sha256.Sum256(index[entry.Name])
			if len(index[entry.Name]) > 8192 || hex.EncodeToString(got[:]) != digests[entry.Name].hash {
				return nil, ErrInvalid
			}
		}
	}
	verified, err := validateFiles(index, func(name string) string { return digests[name].hash }, func(name string, _ []byte, revision string) error {
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		return errors.Join(executable(name, file, revision), file.Close())
	})
	if err != nil {
		return nil, err
	}
	return &DiskBundle{parent: parent, directory: directory, metadata: verified.Metadata(), files: digests}, nil
}

// Stage rechecks the disk contents before publishing. Caller authorization must
// still be held; a failed check never publishes a partially validated directory.
func (b *DiskBundle) Stage(parent *os.Root, directory string) error {
	publish, err := b.PrepareStage(parent, directory)
	if err != nil {
		return err
	}
	return publish()
}

// PrepareStage rechecks all file contents without publishing. The caller must
// exclusively own the bundle and its private directory until publication or
// Close. The returned function only rechecks the target and renames; call it
// under the final authorization fence, or discard it and Close the bundle.
func (b *DiskBundle) PrepareStage(parent *os.Root, directory string) (func() error, error) {
	if err := b.verifyStage(parent, directory); err != nil {
		return nil, err
	}
	source := b.directory
	return func() error {
		if b.directory != source {
			return ErrInvalid
		}
		if _, err := parent.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(ErrInvalid, err)
		}
		if err := parent.Rename(source, directory); err != nil {
			return err
		}
		b.directory = ""
		return nil
	}, nil
}

func (b *DiskBundle) verifyStage(parent *os.Root, directory string) error {
	if b == nil || b.parent != parent || b.directory == "" || !validHex(directory, 32) || directory == b.directory {
		return ErrInvalid
	}
	if _, err := parent.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrInvalid, err)
	}
	root, err := parent.OpenRoot(b.directory)
	if err != nil {
		return err
	}
	listing, err := root.Open(".")
	if err != nil {
		return errors.Join(err, root.Close())
	}
	entries, readErr := listing.ReadDir(len(b.files) + 1)
	closeErr := listing.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) != len(b.files) {
		return errors.Join(ErrInvalid, readErr, closeErr, root.Close())
	}
	for _, entry := range entries {
		if _, known := b.files[entry.Name()]; !known || !entry.Type().IsRegular() {
			return errors.Join(ErrInvalid, root.Close())
		}
	}
	for name, want := range b.files {
		file, err := root.Open(name)
		if err != nil {
			return errors.Join(err, root.Close())
		}
		info, statErr := file.Stat()
		hash := sha256.New()
		n, copyErr := io.Copy(hash, io.LimitReader(file, want.size+1))
		closeErr := file.Close()
		if statErr != nil || info == nil || !info.Mode().IsRegular() || info.Size() != want.size || n != want.size || hex.EncodeToString(hash.Sum(nil)) != want.hash || copyErr != nil || closeErr != nil {
			return errors.Join(ErrInvalid, statErr, copyErr, closeErr, root.Close())
		}
	}
	return root.Close()
}
