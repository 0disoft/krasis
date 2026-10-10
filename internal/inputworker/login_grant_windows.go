//go:build windows

package inputworker

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
)

const loginGrantName = "LOGIN-GRANT.json"

var ErrLoginConfigurationUnconfirmed = errors.New("login configuration change is unconfirmed")

// ReadLoginGrant trusts only the fixed, administrator-owned file object. It
// cannot create/repair a directory or import configuration supplied over IPC.
func ReadLoginGrant(now time.Time) (loginagent.Grant, bool, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return loginagent.Grant{}, false, ErrAuthority
	}
	root = filepath.Join(root, "Krasis")
	parent, err := openProtected(root, true)
	if errors.Is(err, os.ErrNotExist) {
		return loginagent.Grant{}, false, nil
	}
	if err != nil {
		return loginagent.Grant{}, false, err
	}
	defer parent.Close()
	file, err := openProtected(filepath.Join(root, loginGrantName), false)
	if errors.Is(err, os.ErrNotExist) {
		return loginagent.Grant{}, false, nil
	}
	if err != nil {
		return loginagent.Grant{}, false, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 4097))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return loginagent.Grant{}, false, ErrAuthority
	}
	g, err := loginagent.DecodeGrant(data, now)
	if err != nil {
		return loginagent.Grant{}, true, err
	}
	return g, true, nil
}

// ConfigureLoginGrant provisions a reference, not a credential copy or running
// connection. Runtime activation is a separate integration step. No SCM, peer
// connection, executable or user-profile state is changed by this operation.
func ConfigureLoginGrant(ctx context.Context, alias string) (result error) {
	changed := false
	if ctx == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	now := time.Now().UTC()
	grant, err := loginagent.PrepareCurrentGrant(alias, now)
	if err != nil {
		return err
	}
	installation, err := LoadInstallation()
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(result, installation.Close(), changed) }()
	files, err := newInstallFiles(nil)
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(result, files.close(), changed) }()
	unlock, err := files.acquire(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	// Never repurpose or delete an installation transaction to configure boot.
	journal, err := files.load(ctx)
	if err != nil || journal != nil {
		return ErrRecovery
	}
	if _, err := loginagent.ReadGrantConnection(grant, now); err != nil {
		return err
	}
	data, err := loginagent.EncodeGrant(grant, now)
	if err != nil {
		return err
	}
	pending := filepath.Join(files.root, "LOGIN-GRANT.pending")
	if err := writeInstallFile(ctx, pending, data); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	changed = true
	if err := moveInstallPath(pending, filepath.Join(files.root, loginGrantName), true); err != nil {
		return errors.Join(ErrLoginConfigurationUnconfirmed, err)
	}
	actual, exists, err := ReadLoginGrant(time.Now().UTC())
	if err != nil || !exists || actual.ID != grant.ID {
		return ErrLoginConfigurationUnconfirmed
	}
	if _, err := loginagent.ReadGrantConnection(actual, time.Now().UTC()); err != nil {
		return errors.Join(ErrLoginConfigurationUnconfirmed, err)
	}
	return nil
}

// RemoveLoginGrant accepts no alias/path and does not need a valid old grant.
// It can therefore disable expired or malformed configuration without enabling
// any repair. Administrator access and the installation lock remain required.
func RemoveLoginGrant(ctx context.Context) (result error) {
	changed := false
	if ctx == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	files, err := newInstallFiles(nil)
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(result, files.close(), changed) }()
	unlock, err := files.acquire(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	dir, err := securefs.OpenDirectory(files.root)
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(result, dir.Close(), changed) }()
	if protectedHandle(windows.Handle(dir.Handle())) != nil {
		return ErrAuthority
	}
	file, err := dir.Open(loginGrantName)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := protectedHandle(windows.Handle(file.Fd())); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	changed = true
	if err := dir.Remove(loginGrantName); err != nil {
		return errors.Join(ErrLoginConfigurationUnconfirmed, err)
	}
	remaining, err := dir.Open(loginGrantName)
	if remaining != nil {
		_ = remaining.Close()
		return ErrLoginConfigurationUnconfirmed
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ErrLoginConfigurationUnconfirmed
	}
	return nil
}

func closeLoginConfiguration(result, closeErr error, changed bool) error {
	if closeErr == nil {
		return result
	}
	if changed {
		return errors.Join(ErrLoginConfigurationUnconfirmed, result, closeErr)
	}
	return errors.Join(result, closeErr)
}
