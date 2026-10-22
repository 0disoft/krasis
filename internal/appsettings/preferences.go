package appsettings

import (
	"errors"
	"os"
	"sync"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

// PreferencesLease serializes saved preferences and their OS registrations
// across GUI/setup processes. Acquire it before reading the settings to change.
// Registration helpers do not reacquire it; callers keep it through rollback.
type PreferencesLease struct {
	mu     sync.Mutex
	base   string
	unlock func() error
}

func LockPreferences(base string) (*PreferencesLease, error) {
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		return nil, err
	}
	file, err := openPreferencesLock(dir)
	if err != nil {
		dir.Close()
		return nil, err
	}
	unlock, err := lockPreferencesFile(file)
	if err != nil {
		file.Close()
		dir.Close()
		return nil, err
	}
	return &PreferencesLease{base: base, unlock: func() error {
		return errors.Join(unlock(), file.Close(), dir.Close())
	}}, nil
}

func openPreferencesLock(dir *securefs.Directory) (*os.File, error) {
	file, err := dir.CreateExclusive(".preferences.lock", 0600)
	if errors.Is(err, os.ErrExist) {
		file, err = dir.OpenReadWrite(".preferences.lock")
	} else if err == nil {
		err = peercredentials.ProtectSecurityFileHandle(file)
	}
	if err == nil {
		err = peercredentials.ValidateSecurityFileHandle(file)
	}
	if err != nil {
		if file != nil {
			file.Close()
		}
		return nil, err
	}
	return file, nil
}

func (p *PreferencesLease) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unlock == nil {
		return nil
	}
	unlock := p.unlock
	p.unlock = nil
	return unlock()
}

func (p *PreferencesLease) Save(value Settings) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unlock == nil {
		return os.ErrClosed
	}
	return saveSettings(p.base, value)
}

func (p *PreferencesLease) Initialize() (Settings, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unlock == nil {
		return Settings{}, os.ErrClosed
	}
	return initializeSettings(p.base)
}
