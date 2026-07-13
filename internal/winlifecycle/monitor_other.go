//go:build !windows

package winlifecycle

import (
	"errors"

	"github.com/0disoft/krasis/internal/agentlifecycle"
)

func StartMonitor() (agentlifecycle.Monitor, error) {
	return nil, errors.New("Windows lifecycle monitoring requires Windows")
}
