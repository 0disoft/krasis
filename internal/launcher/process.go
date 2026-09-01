package launcher

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/0disoft/krasis/internal/securefs"
)

type OperationResult struct {
	Command  string
	ExitCode int
	Err      error
}

type Manager struct {
	mu       sync.Mutex
	running  bool
	stopFile string
}

func (manager *Manager) Start(
	executable string,
	workingDirectory string,
	commands []CommandSpec,
	stopFile string,
	output func(string),
	complete func(OperationResult),
) error {
	if executable == "" || workingDirectory == "" || len(commands) == 0 {
		return errors.New("executable, working directory, and commands are required")
	}
	manager.mu.Lock()
	if manager.running {
		manager.mu.Unlock()
		return errors.New("another launcher operation is already running")
	}
	manager.running = true
	manager.stopFile = stopFile
	manager.mu.Unlock()

	go func() {
		result := OperationResult{ExitCode: 0}
		for _, command := range commands {
			result.Command = command.Name
			cmd := exec.Command(executable, command.Args...)
			cmd.Dir = workingDirectory
			configureProcess(cmd)
			writer := callbackWriter{callback: output}
			cmd.Stdout = writer
			cmd.Stderr = writer
			if output != nil {
				output("[" + command.Name + "] 시작\r\n")
			}
			err := cmd.Run()
			if err != nil {
				result.Err = err
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					result.ExitCode = exitErr.ExitCode()
				} else {
					result.ExitCode = -1
				}
				break
			}
		}
		manager.mu.Lock()
		manager.running = false
		manager.stopFile = ""
		manager.mu.Unlock()
		if complete != nil {
			complete(result)
		}
	}()
	return nil
}

func (manager *Manager) Running() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.running
}

func (manager *Manager) RequestStop() error {
	manager.mu.Lock()
	if !manager.running {
		manager.mu.Unlock()
		return errors.New("no launcher operation is running")
	}
	path := manager.stopFile
	manager.mu.Unlock()
	if path == "" {
		return errors.New("the current operation cannot be stopped safely")
	}
	directory, err := securefs.OpenDirectory(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open stop signal directory: %w", err)
	}
	defer directory.Close()
	file, err := directory.CreateExclusive(filepath.Base(path), 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create stop signal: %w", err)
	}
	if _, err := io.WriteString(file, "stop\n"); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

type callbackWriter struct {
	callback func(string)
}

func (writer callbackWriter) Write(payload []byte) (int, error) {
	if writer.callback != nil && len(payload) > 0 {
		writer.callback(string(payload))
	}
	return len(payload), nil
}
