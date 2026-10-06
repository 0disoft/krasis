//go:build windows

package appsettings

import (
	"strings"
	"testing"
)

func TestStartupUsesQuotedStableLauncher(t *testing.T) {
	cmd := StartupCommand(`C:\Users\Test User\AppData\Local`)
	if !strings.HasPrefix(cmd, `"C:\Users\Test User\`) || !strings.HasSuffix(cmd, `krasis-cli.exe" relaunch --startup`) {
		t.Fatal(cmd)
	}
}
