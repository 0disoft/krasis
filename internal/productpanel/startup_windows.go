//go:build windows

package productpanel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
)

func NewStartup(control string) (Startup, error) {
	self, err := os.Executable()
	if err != nil {
		return Startup{}, err
	}
	base := filepath.Dir(self)
	revision := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) == 40 {
				if _, err := hex.DecodeString(setting.Value); err == nil {
					revision = setting.Value
				}
			}
		}
	}
	refresh := newInputRefresh(base, revision)
	return Startup{
		BeforeStart:       refresh.automatic,
		InputRefresh:      refresh.explicit,
		InputSetup:        refresh.status,
		InputRecheck:      refresh.recheck,
		InputRestoreCheck: refresh.restoreCheck,
		Open:              func() (RuntimeClient, error) { return localapi.OpenClient(control) },
		Lock:              func() (io.Closer, error) { return agentinstance.AcquireControlStartup() },
		Available: func() (bool, error) {
			lease, err := agentinstance.Acquire()
			if errors.Is(err, agentinstance.ErrAlreadyRunning) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return true, lease.Close()
		},
		Verify: func() error { return verifyAgentBundle(base, filepath.Base(self)) },
		Start: func() error {
			if err := verifyAgentBundle(base, filepath.Base(self)); err != nil {
				return err
			}
			agentPath := filepath.Join(base, "krasis-agent.exe")
			data, err := os.ReadFile(agentPath)
			if err != nil {
				return err
			}
			installed, installErr := inputworker.OpenAgentLaunch(sha256.Sum256(data))
			if installErr == nil && installed != nil {
				defer installed.Close()
				agentPath = installed.Path()
			}
			command := exec.Command(agentPath)
			command.Dir = base
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
			for _, key := range []string{"SystemRoot", "WINDIR", "LOCALAPPDATA", "TEMP", "TMP", "USERPROFILE"} {
				if value, ok := os.LookupEnv(key); ok {
					command.Env = append(command.Env, key+"="+value)
				}
			}
			// Detached agent owns its own single-instance lease and safe-stop lifecycle.
			return launchrecord.StartIndependent(command)
		},
		Pause: startupPause, Generation: localapi.AgentGeneration, Revision: revision,
	}, nil
}

func verifyAgentBundle(base, self string) error {
	read := func(name string, limit int64) ([]byte, error) {
		path := filepath.Join(base, name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > limit {
			return nil, errors.New("invalid bundle file")
		}
		return os.ReadFile(path)
	}
	manifest, err := read("SHA256SUMS.txt", 8192)
	if err != nil {
		return err
	}
	for _, name := range []string{self, "krasis-agent.exe"} {
		count, want := 0, ""
		for _, line := range strings.Split(string(manifest), "\n") {
			parts := strings.SplitN(strings.TrimSuffix(line, "\r"), "  ", 2)
			if len(parts) == 2 && parts[1] == name {
				count++
				want = parts[0]
			}
		}
		if count != 1 || len(want) != 64 {
			return errors.New("missing bundle checksum")
		}
		data, err := read(name, 64<<20)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != want {
			return errors.New("bundle checksum mismatch")
		}
	}
	return nil
}
