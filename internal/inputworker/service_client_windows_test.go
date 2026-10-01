//go:build windows

package inputworker

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestLimitedServiceProcessInspectionNeedsNoTokenOrSynchronize(t *testing.T) {
	identity, err := inspectServiceProcess(windows.GetCurrentProcessId())
	if err != nil {
		t.Fatal("limited process query failed")
	}
	defer identity.close()
	if identity.handle == 0 || identity.image == "" || identity.token != 0 {
		t.Fatal("service client acquired token or lost kernel facts")
	}
	if value, err := inspectServiceProcess(0); value != nil || err != ErrAuthority {
		t.Fatal("missing process accepted")
	}
}
