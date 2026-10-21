//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const installFileACL = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FRFX;;;BU)"

var installFileNames = [...]string{"krasis-agent.exe", "krasis-input.exe", "INPUT-INSTALL.json"}

type installDirectory uint8

const (
	installActive installDirectory = iota
	installCandidate
	installBackup
	installRetired
)

type installFiles struct {
	root    string
	pinned  *os.File
	lock    *os.File
	payload *installPayload
}

// This is the only production root selection. It does not read environment or
// request paths and cannot elevate its caller or repair an unsafe existing ACL.
func newInstallFiles(payload *installPayload) (*installFiles, error) {
	if payload != nil && !payload.valid() {
		return nil, ErrAuthority
	}
	identity, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return nil, ErrAuthority
	}
	defer identity.close()
	if identity.restricted || identity.appContainer || identity.integrity < 0x3000 || !identity.token.IsElevated() {
		return nil, ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, installNativeError("known_folder", ErrAuthority, err)
	}
	root = filepath.Join(root, "Krasis")
	if err := createInstallDirectory(root); err != nil {
		return nil, err
	}
	pinned, err := openProtected(root, true)
	if err != nil {
		return nil, err
	}
	return &installFiles{root: root, pinned: pinned, payload: payload}, nil
}

func (files *installFiles) directory(kind installDirectory) string {
	names := [...]string{"Input", "Input.stage", "Input.rollback", "Input.retired"}
	if files == nil || kind > installRetired {
		return ""
	}
	return filepath.Join(files.root, names[kind])
}

func installAttributes() (*windows.SecurityAttributes, error) {
	descriptor, err := windows.SecurityDescriptorFromString(installFileACL)
	if err != nil {
		return nil, ErrAuthority
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}, nil
}

func createInstallDirectory(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ErrAuthority
	}
	attributes, err := installAttributes()
	if err != nil {
		return err
	}
	if err := windows.CreateDirectory(name, attributes); err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return installNativeError("directory_create", ErrAuthority, err)
	}
	file, err := openProtected(path, true)
	if err != nil {
		return err
	}
	return file.Close()
}

// OPEN_ALWAYS does not truncate an existing object before its owner, DACL and
// type are checked. Exclusive sharing also pins its identity while it is written.
func openInstallWrite(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, ErrAuthority
	}
	attributes, err := installAttributes()
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL, 0, attributes, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_WRITE_THROUGH, 0)
	if err != nil {
		return nil, installNativeError("file_open", ErrAuthority, err)
	}
	file := os.NewFile(uintptr(handle), "protected-input-write")
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = file.Close()
		return nil, installNativeError("file_attributes", ErrAuthority, err)
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		_ = file.Close()
		return nil, ErrAuthority
	}
	if err := protectedHandle(handle); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func (files *installFiles) acquire(ctx context.Context) (func(), error) {
	if files == nil || files.pinned == nil || files.lock != nil || ctx.Err() != nil {
		return nil, ErrAuthority
	}
	file, err := openInstallWrite(filepath.Join(files.root, "INPUT-TRANSACTION.lock"))
	if err != nil {
		return nil, err
	}
	files.lock = file
	return func() { _ = files.lock.Close(); files.lock = nil }, nil
}

func (files *installFiles) check(ctx context.Context) error {
	if files == nil || files.pinned == nil || files.lock == nil || ctx == nil || ctx.Err() != nil || protectedHandle(windows.Handle(files.pinned.Fd())) != nil {
		return ErrAuthority
	}
	return nil
}

func (files *installFiles) load(ctx context.Context) (*installJournal, error) {
	if err := files.check(ctx); err != nil {
		return nil, err
	}
	path := filepath.Join(files.root, "INPUT-TRANSACTION.json")
	if missingInstallPath(path) {
		for _, kind := range []installDirectory{installCandidate, installBackup, installRetired} {
			if !missingInstallPath(files.directory(kind)) {
				return nil, errInstallRecovery
			}
		}
		return nil, nil
	}
	file, err := openProtected(path, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil {
		return nil, installNativeError("file_read", errInstallRecovery, err)
	}
	journal, err := decodeInstallJournal(data)
	if err != nil {
		return nil, err
	}
	return &journal, nil
}

func (files *installFiles) save(ctx context.Context, journal installJournal) error {
	if err := files.check(ctx); err != nil {
		return err
	}
	data, err := encodeInstallJournal(journal)
	if err != nil {
		return err
	}
	pending := filepath.Join(files.root, "INPUT-TRANSACTION.pending")
	if err := writeInstallFile(ctx, pending, data); err != nil {
		return err
	}
	destination := filepath.Join(files.root, "INPUT-TRANSACTION.json")
	if !missingInstallPath(destination) {
		file, err := openProtected(destination, false)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return installNativeError("file_close", ErrAuthority, err)
		}
	}
	if ctx.Err() != nil {
		return ErrAuthority
	}
	return moveInstallPath(pending, destination, true)
}

func writeInstallFile(ctx context.Context, path string, data []byte) error {
	if ctx.Err() != nil {
		return ErrAuthority
	}
	file, err := openInstallWrite(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Truncate(0); err != nil {
		return installNativeError("file_write", ErrAuthority, err)
	}
	reader := &contextInstallReader{ctx: ctx, reader: bytes.NewReader(data)}
	if _, err := io.Copy(file, reader); err != nil {
		return installNativeError("file_write", ErrAuthority, err)
	}
	if err := file.Sync(); err != nil {
		return installNativeError("file_sync", ErrAuthority, err)
	}
	if err := file.Close(); err != nil {
		return installNativeError("file_close", ErrAuthority, err)
	}
	return nil
}

type contextInstallReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextInstallReader) Read(data []byte) (int, error) {
	if reader.ctx.Err() != nil {
		return 0, ErrAuthority
	}
	if len(data) > 64<<10 {
		data = data[:64<<10]
	}
	return reader.reader.Read(data)
}

func missingInstallPath(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, os.ErrNotExist)
}

func moveInstallPath(from, to string, replace bool) error {
	return moveInstallPathOperation(from, to, replace, "file_move")
}

func moveInstallPathOperation(from, to string, replace bool, operation string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return ErrAuthority
	}
	destination, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return ErrAuthority
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replace {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	if err := windows.MoveFileEx(source, destination, flags); err != nil {
		return installNativeError(operation, ErrAuthority, err)
	}
	return nil
}

func (files *installFiles) writeImages(ctx context.Context, kind installDirectory, images installImages, agent, input []byte) error {
	if err := files.check(ctx); err != nil {
		return err
	}
	if !images.valid() || !missingInstallPath(files.directory(kind)) {
		return ErrAuthority
	}
	if err := createInstallDirectory(files.directory(kind)); err != nil {
		return err
	}
	record, err := json.Marshal(installationRecord{Schema: "krasis.input.install/v1", Agent: hex.EncodeToString(images.Agent[:]), Service: hex.EncodeToString(images.Service[:])})
	if err != nil {
		return ErrAuthority
	}
	for index, data := range [][]byte{agent, input, record} {
		if err := writeInstallFile(ctx, filepath.Join(files.directory(kind), installFileNames[index]), data); err != nil {
			return err
		}
	}
	verified, err := files.readImages(ctx, kind, &images)
	if err != nil {
		return err
	}
	return verified.Close()
}

func (files *installFiles) readImages(ctx context.Context, kind installDirectory, expected *installImages) (*Installation, error) {
	if err := files.check(ctx); err != nil {
		return nil, err
	}
	installation, err := loadInstallationDirectory(files.root, files.directory(kind))
	if err != nil {
		return nil, err
	}
	if expected != nil && (installation.agentDigest != expected.Agent || installation.serviceDigest != expected.Service) {
		_ = installation.Close()
		return nil, ErrAuthority
	}
	return installation, nil
}

func (files *installFiles) stage(ctx context.Context, journal installJournal) error {
	if err := files.check(ctx); err != nil || !journal.valid() {
		return ErrAuthority
	}
	if journal.Previous.Present {
		old, err := files.readImages(ctx, installActive, &journal.Previous.Images)
		if err != nil {
			return err
		}
		defer old.Close()
		var images [2][]byte
		for index, file := range old.files[3:5] {
			images[index], err = io.ReadAll(&contextInstallReader{ctx: ctx, reader: io.NewSectionReader(file, 0, 60<<20+1)})
			if err != nil || len(images[index]) > 60<<20 {
				return ErrAuthority
			}
		}
		if err := files.writeImages(ctx, installBackup, journal.Previous.Images, images[0], images[1]); err != nil {
			return err
		}
	}
	if journal.Target == (installImages{}) {
		return nil
	}
	if !files.payload.valid() || files.payload.images != journal.Target {
		return ErrAuthority
	}
	return files.writeImages(ctx, installCandidate, journal.Target, files.payload.agent, files.payload.input)
}

func (files *installFiles) close() error {
	if files == nil {
		return nil
	}
	var result error
	if files.lock != nil {
		result = files.lock.Close()
		files.lock = nil
	}
	if files.pinned != nil {
		if err := files.pinned.Close(); err != nil {
			result = installNativeError("file_close", ErrAuthority, err)
		}
		files.pinned = nil
	}
	return result
}
