//go:build windows

package productpanel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/securefs"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
)

func NewStartup(control string, recovery ...string) (Startup, error) {
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
	var checkStart func() error
	var agentArgs []string
	if len(recovery) != 0 {
		token := recovery[0]
		checkStart = func() error {
			if !appsettings.RecoveryCurrent(os.Getenv("LOCALAPPDATA"), token) {
				return errors.New("automatic recovery superseded")
			}
			return nil
		}
		agentArgs = []string{"--recovery", token}
	}
	return Startup{
		CheckStart:        checkStart,
		BeforeStart:       refresh.automatic,
		InputRefresh:      refresh.explicit,
		InputSetup:        refresh.status,
		InputAttempt:      refresh.attemptResult,
		InputRecheck:      refresh.recheck,
		InputProbe:        refresh.probe,
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
			installed, installErr := inputworker.OpenAgentLaunchForStartup(sha256.Sum256(data))
			if installErr == nil && installed != nil {
				defer installed.Close()
				agentPath = installed.Path()
			}
			command := exec.Command(agentPath, agentArgs...)
			command.Dir = base
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
			for _, key := range []string{"SystemRoot", "WINDIR", "LOCALAPPDATA", "TEMP", "TMP", "USERPROFILE"} {
				if value, ok := os.LookupEnv(key); ok {
					command.Env = append(command.Env, key+"="+value)
				}
			}
			if os.Getenv("KRASIS_RUNTIME_TRACE") == "1" {
				command.Env = append(command.Env, "KRASIS_RUNTIME_TRACE=1")
			}
			// Detached agent owns its own single-instance lease and safe-stop lifecycle.
			return launchrecord.StartIndependent(command)
		},
		Pause: startupPause, Generation: localapi.AgentGeneration, Revision: revision,
	}, nil
}

func verifyAgentBundle(base, self string) error {
	directory, err := securefs.OpenReadDirectory(base)
	if err != nil {
		return err
	}
	defer directory.Close()
	open := func(name string, limit int64) (*os.File, error) {
		file, err := directory.Open(name)
		if err != nil {
			return nil, err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > limit {
			_ = file.Close()
			return nil, errStartupBundleFile
		}
		return file, nil
	}
	manifestFile, err := open("SHA256SUMS.txt", 8192)
	if err != nil {
		return err
	}
	manifest, readErr := io.ReadAll(io.LimitReader(manifestFile, 8193))
	if err := errors.Join(readErr, manifestFile.Close()); err != nil {
		return err
	}
	if len(manifest) > 8192 {
		return errStartupBundleFile
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
		file, err := open(name, maxStartupImageBytes)
		if err != nil {
			return err
		}
		hash, hashErr := hashStartupBundleImage(file)
		if err := errors.Join(hashErr, file.Close()); err != nil {
			return err
		}
		if hex.EncodeToString(hash[:]) != want {
			return errors.New("bundle checksum mismatch")
		}
	}
	return nil
}
