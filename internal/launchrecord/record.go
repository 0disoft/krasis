// Package launchrecord remembers the last successfully opened product UI.
// It accepts no caller-selected executable, arguments, or remote paths.
package launchrecord

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

const schema = "krasis.launch/v1"
const maxExecutable = 64 << 20

var ErrMissing = errors.New("launch record missing")
var ErrInvalid = errors.New("launch record invalid")
var ErrOlder = errors.New("newer launch record already registered")

type Record struct {
	Schema     string    `json:"schema"`
	Executable string    `json:"executable"`
	SHA256     string    `json:"sha256"`
	Generation int       `json:"generation"`
	Revision   string    `json:"revision"`
	RecordedAt time.Time `json:"recordedAt"`
}

func Directory(localAppData string) (string, error) {
	paths, err := productidentity.ResolveWindowsPaths(localAppData)
	if err != nil {
		return "", err
	}
	return filepath.Join(paths.Root, "launch"), nil
}

func validExecutable(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path &&
		!strings.HasPrefix(path, `\\`) && strings.EqualFold(filepath.Base(path), "krasis.exe")
}

func openExecutable(path string) (*os.File, string, error) {
	if !validExecutable(path) {
		return nil, "", ErrInvalid
	}
	dir, err := securefs.OpenReadDirectory(filepath.Dir(path))
	if err != nil {
		return nil, "", err
	}
	defer dir.Close()
	file, err := dir.Open(filepath.Base(path))
	if err != nil {
		return nil, "", err
	}
	hash, err := digest(file)
	if err != nil {
		file.Close()
		return nil, "", err
	}
	return file, hash, nil
}

func digest(file *os.File) (string, error) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxExecutable {
		return "", ErrInvalid
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(file, maxExecutable+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Register(localAppData, executable string, generation int, revision string) error {
	file, hash, err := openExecutable(executable)
	if err != nil {
		return registrationFailure("image", err)
	}
	defer file.Close()
	if generation < 1 {
		return registrationFailure("metadata", ErrInvalid)
	}
	dir, err := productidentity.PrepareLaunch(localAppData)
	if err != nil {
		return registrationFailure("directory", err)
	}
	defer dir.Close()
	unlock, err := lockRegistration(dir)
	if err != nil {
		return registrationFailure("record", err)
	}
	defer unlock()
	previous, err := Load(localAppData)
	if err == nil && previous.Generation > generation {
		return registrationFailure("version", ErrOlder)
	}
	if err != nil && !errors.Is(err, ErrMissing) {
		return registrationFailure("record", err)
	}
	data, err := json.Marshal(Record{schema, executable, hash, generation, revision, time.Now().UTC()})
	if err != nil {
		return registrationFailure("metadata", err)
	}
	if previous.Schema == schema && previous.Executable != executable {
		backup, err := json.Marshal(previous)
		if err != nil {
			return registrationFailure("record", err)
		}
		if err = replace(dir, "previous.json", backup); err != nil {
			return registrationFailure("record", err)
		}
	}
	// Publish the current record only after both fixed recovery entrypoints exist.
	if err = copyCLI(dir, filepath.Dir(executable)); err != nil {
		return registrationFailure("cli", err)
	}
	if err = replace(dir, "Krasis.cmd", []byte("@echo off\r\n\"%~dp0krasis-cli.exe\" relaunch\r\nif errorlevel 1 pause\r\n")); err != nil {
		return registrationFailure("script", err)
	}
	if err = replace(dir, "current.json", data); err != nil {
		return registrationFailure("record", err)
	}
	return nil
}

func copyCLI(destination *securefs.Directory, source string) error {
	dir, err := securefs.OpenReadDirectory(source)
	if err != nil {
		return err
	}
	defer dir.Close()
	file, err := dir.Open("krasis-cli.exe")
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() <= 0 || info.Size() > maxExecutable {
		return ErrInvalid
	}
	// Hash an unchanged recovery CLI without retaining the whole executable.
	// The pinned source handle stays open through comparison and replacement.
	if old, err := destination.Open("krasis-cli.exe"); err == nil {
		hash, readErr := digest(old)
		old.Close()
		if readErr == nil {
			want, err := digest(file)
			if err != nil {
				return err
			}
			if hash == want {
				return nil
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
		}
	}
	return replaceWith(destination, "krasis-cli.exe", func(output *os.File) error {
		return copyCLIStream(output, file, info.Size())
	})
}

func replace(dir *securefs.Directory, name string, data []byte) error {
	return replaceWith(dir, name, func(file *os.File) error {
		n, err := file.Write(data)
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
		return err
	})
}

func replaceWith(dir *securefs.Directory, name string, write func(*os.File) error) error {
	file, temporary, err := dir.CreateTemporary(".launch-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(temporary)
	if err = peercredentials.ProtectSecurityFileHandle(file); err == nil {
		err = write(file)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return dir.Replace(temporary, name)
}

func Load(localAppData string) (Record, error) {
	return loadNamed(localAppData, "current.json")
}

func loadNamed(localAppData, name string) (Record, error) {
	path, err := Directory(localAppData)
	if err != nil {
		return Record{}, err
	}
	if _, err = os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrMissing
	}
	if _, err = peercredentials.ValidateSecurityDirectory(path); err != nil {
		return Record{}, err
	}
	dir, err := securefs.OpenReadDirectory(path)
	if err != nil {
		return Record{}, err
	}
	defer dir.Close()
	file, err := dir.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrMissing
	}
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	if err = peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return Record{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(data) > 8192 {
		return Record{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record Record
	if decoder.Decode(&record) != nil || record.Schema != schema || !validExecutable(record.Executable) ||
		record.Generation < 1 || record.RecordedAt.IsZero() || len(record.SHA256) != 64 {
		return Record{}, ErrInvalid
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return Record{}, ErrInvalid
	}
	return record, nil
}

// RecoveryDirectories preserves recent launches and the actual recovery task's
// last successful target. An unreadable
// record prevents cleanup rather than making its image eligible for deletion.
func RecoveryDirectories(base string) ([]string, error) {
	var paths []string
	for _, name := range []string{"current.json", "previous.json", recoveryTaskRecord} {
		record, err := loadNamed(base, name)
		if errors.Is(err, ErrMissing) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		path := filepath.Dir(record.Executable)
		found := false
		for _, prior := range paths {
			if strings.EqualFold(prior, path) {
				found = true
				break
			}
		}
		if !found {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// Inspect verifies the remembered image without launching or modifying it.
func Inspect(localAppData string) (Record, error) {
	record, err := Load(localAppData)
	if err != nil {
		return Record{}, err
	}
	file, hash, err := openExecutable(record.Executable)
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	if hash != record.SHA256 {
		return Record{}, ErrInvalid
	}
	return record, nil
}

// Relaunch verifies the recorded image. Fallback is only for first-time setup.
func Relaunch(localAppData, fallback string, start func(string) error) (Record, error) {
	record, err := Load(localAppData)
	if errors.Is(err, ErrMissing) && fallback != "" {
		record.Executable = fallback
	} else if err != nil {
		return Record{}, err
	}
	file, hash, err := openExecutable(record.Executable)
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	if record.SHA256 != "" && hash != record.SHA256 {
		return Record{}, ErrInvalid
	}
	if start == nil {
		start = Start
	}
	if err = start(record.Executable); err != nil {
		return Record{}, err
	}
	return record, nil
}
