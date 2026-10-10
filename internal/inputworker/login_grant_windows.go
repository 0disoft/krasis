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
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
)

const (
	loginGrantName    = "LOGIN-GRANT.json"
	loginGrantPending = "LOGIN-GRANT.pending"
)

var ErrLoginConfigurationUnconfirmed = errors.New("login configuration change is unconfirmed")

// ReadLoginGrant trusts only the fixed, administrator-owned file object. It
// cannot create/repair a directory or import configuration supplied over IPC.
func ReadLoginGrant(now time.Time) (grant loginagent.Grant, present bool, result error) {
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
	defer func() {
		if err := parent.Close(); err != nil {
			grant, result = loginagent.Grant{}, errors.Join(ErrLoginConfigurationUnconfirmed, result)
		}
	}()
	// A crash before grant replacement must not silently resurrect an older
	// committed policy. Only an explicit configure/disable can retire pending.
	pending, err := openProtected(filepath.Join(root, loginGrantPending), false)
	if pending != nil {
		_ = pending.Close()
		return loginagent.Grant{}, false, ErrLoginConfigurationUnconfirmed
	}
	if !errors.Is(err, os.ErrNotExist) {
		return loginagent.Grant{}, false, ErrLoginConfigurationUnconfirmed
	}
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
	stage := "prepare"
	defer func() { result = logindiag.At(stage, result) }()
	changed := false
	if ctx == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	now := time.Now().UTC()
	grant, err := loginagent.PrepareCurrentGrant(alias, now)
	if err != nil {
		return err
	}
	stage = "installation_read"
	installation, err := LoadInstallation()
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(logindiag.At(stage, result), installation.Close(), changed) }()
	stage = "install_files"
	files, err := newInstallFiles(nil)
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(logindiag.At(stage, result), files.close(), changed) }()
	stage = "install_lock"
	if err := files.lockLoginConfiguration(ctx); err != nil {
		return err
	}
	// Never repurpose or delete an installation transaction to configure boot.
	stage = "transaction_read"
	journal, err := files.load(ctx)
	if err != nil {
		return errors.Join(ErrRecovery, err)
	}
	if journal != nil {
		return logindiag.At("transaction_present", ErrRecovery)
	}
	stage = "policy_recheck"
	if _, err := loginagent.ReadGrantConnection(grant, now); err != nil {
		return err
	}
	stage = "grant_encode"
	data, err := loginagent.EncodeGrant(grant, now)
	if err != nil {
		return err
	}
	pending := filepath.Join(files.root, loginGrantPending)
	stage = "pending_write"
	if err := writeInstallFile(ctx, pending, data); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	changed = true
	stage = "commit"
	if err := moveInstallPath(pending, filepath.Join(files.root, loginGrantName), true); err != nil {
		return errors.Join(ErrLoginConfigurationUnconfirmed, err)
	}
	stage = "grant_verify"
	actual, exists, err := ReadLoginGrant(time.Now().UTC())
	if err != nil || !exists || actual.ID != grant.ID {
		return errors.Join(ErrLoginConfigurationUnconfirmed, err)
	}
	stage = "committed_policy"
	if _, err := loginagent.ReadGrantConnection(actual, time.Now().UTC()); err != nil {
		return errors.Join(ErrLoginConfigurationUnconfirmed, err)
	}
	return nil
}

// RemoveLoginGrant accepts no alias/path and does not need a valid old grant.
// It can therefore disable expired or malformed configuration without enabling
// any repair. Administrator access and the installation lock remain required.
func RemoveLoginGrant(ctx context.Context) (result error) {
	defer func() { result = logindiag.At("remove", result) }()
	changed := false
	if ctx == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	files, err := newInstallFiles(nil)
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(result, files.close(), changed) }()
	if err := files.lockLoginConfiguration(ctx); err != nil {
		return err
	}
	dir, err := securefs.OpenDirectory(files.root)
	if err != nil {
		return err
	}
	defer func() { result = closeLoginConfiguration(result, dir.Close(), changed) }()
	if protectedHandle(windows.Handle(dir.Handle())) != nil {
		return ErrAuthority
	}
	for _, name := range []string{loginGrantName, loginGrantPending} {
		file, err := dir.Open(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return loginConfigurationFailure(err, changed)
		}
		if err := protectedHandle(windows.Handle(file.Fd())); err != nil {
			_ = file.Close()
			return loginConfigurationFailure(err, changed)
		}
		if err := file.Close(); err != nil {
			return loginConfigurationFailure(err, changed)
		}
		if ctx.Err() != nil {
			return loginConfigurationFailure(ctx.Err(), changed)
		}
		changed = true
		if err := dir.Remove(name); err != nil {
			return errors.Join(ErrLoginConfigurationUnconfirmed, err)
		}
	}
	for _, name := range []string{loginGrantName, loginGrantPending} {
		remaining, err := dir.Open(name)
		if remaining != nil {
			_ = remaining.Close()
			return ErrLoginConfigurationUnconfirmed
		}
		if !errors.Is(err, os.ErrNotExist) {
			return ErrLoginConfigurationUnconfirmed
		}
	}
	return nil
}

func (files *installFiles) lockLoginConfiguration(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || files == nil || files.pinned == nil || files.lock != nil {
		return ErrAuthority
	}
	lock, err := openInstallWrite(filepath.Join(files.root, "INPUT-TRANSACTION.lock"))
	if err != nil {
		return err
	}
	// files.close owns and reports lock close too; the installer's void release
	// callback would otherwise discard that failure after changing configuration.
	files.lock = lock
	return nil
}

func closeLoginConfiguration(result, closeErr error, changed bool) error {
	if closeErr == nil {
		return result
	}
	return loginConfigurationFailure(errors.Join(result, logindiag.At("close", closeErr)), changed)
}

func loginConfigurationFailure(cause error, changed bool) error {
	if changed {
		return errors.Join(ErrLoginConfigurationUnconfirmed, cause)
	}
	return cause
}
