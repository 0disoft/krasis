//go:build !windows

package launchrecord

import (
	"github.com/0disoft/krasis/internal/securefs"
	"sync"
)

var registrationGate sync.Mutex

func lockRegistration(*securefs.Directory) (func(), error) {
	registrationGate.Lock()
	return registrationGate.Unlock, nil
}
