package application

import (
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestLeaseWatchDoesNotAcquireRuntimeMutex(t *testing.T) {
	runtime := &SessionRuntime{control: control.InitialSnapshot()}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.publishLeaseWatchLocked(false)
	if runtime.LeaseWatch().State != control.StateLocal {
		t.Fatal("watch state")
	}
}
