//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestInstallServiceConfigRejectsOtherPathAccountAndLifecycle(t *testing.T) {
	command := `"C:\Program Files\Krasis\Input\krasis-input.exe" --service`
	valid := installServiceConfig{typeID: windows.SERVICE_WIN32_OWN_PROCESS, start: windows.SERVICE_DEMAND_START, errorControl: windows.SERVICE_ERROR_NORMAL, binary: command, account: "LocalSystem"}
	if !validInstallServiceConfig(valid, command) {
		t.Fatal("fixed service config rejected")
	}
	for _, change := range []func(*installServiceConfig){
		func(c *installServiceConfig) { c.binary = `C:\Users\Public\krasis-input.exe --service` },
		func(c *installServiceConfig) { c.account = "LocalService" },
		func(c *installServiceConfig) { c.typeID |= windows.SERVICE_INTERACTIVE_PROCESS },
		func(c *installServiceConfig) { c.start = windows.SERVICE_AUTO_START },
		func(c *installServiceConfig) { c.group = "custom" },
		func(c *installServiceConfig) { c.deps = "other-service" },
		func(c *installServiceConfig) { c.tag = 1 },
	} {
		config := valid
		change(&config)
		if validInstallServiceConfig(config, command) {
			t.Fatal("mutable service config accepted")
		}
	}
}

func TestInstallServiceDescriptorRestrictsUserControl(t *testing.T) {
	for _, example := range []struct {
		text string
		safe bool
	}{
		{"O:BAG:BAD:P(A;;0xF01FF;;;SY)(A;;0xF01FF;;;BA)(A;;0x20005;;;BU)", true},
		{"O:SYG:SYD:P(A;;0xF01FF;;;SY)(A;;0x20085;;;BU)", true},
		{"O:SYG:SYD:P(A;;0x20015;;;BU)", false},
		{"O:SYG:SYD:P(A;;0x20025;;;BU)", false},
		{"O:SYG:SYD:P(A;;0x20007;;;BU)", false},
		{"O:SYG:SYD:P(A;;0x20105;;;BU)", false},
		{"O:BUG:BUD:P(A;;0x20005;;;BU)", false},
		{"O:SYG:SYD:NO_ACCESS_CONTROL", false},
	} {
		descriptor, err := windows.SecurityDescriptorFromString(example.text)
		if err != nil {
			t.Fatal("invalid test descriptor")
		}
		if (protectedInstallService(descriptor) == nil) != example.safe {
			t.Fatal("service ACL classification mismatch")
		}
	}
}

func TestInstallServiceStringParserBoundsKernelPointers(t *testing.T) {
	buffer := make([]byte, 32)
	binary.LittleEndian.PutUint16(buffer[4:6], 'A')
	binary.LittleEndian.PutUint16(buffer[6:8], 'B')
	text, err := installServiceString(buffer, (*uint16)(unsafe.Pointer(&buffer[4])))
	if err != nil || text != "AB" {
		t.Fatal("bounded UTF16 parse failed")
	}
	outside := make([]byte, 4)
	if _, err := installServiceString(buffer, (*uint16)(unsafe.Pointer(&outside[0]))); err != ErrAuthority {
		t.Fatal("outside pointer accepted")
	}
	if _, err := installServiceString(buffer, (*uint16)(unsafe.Pointer(&buffer[3]))); err != ErrAuthority {
		t.Fatal("unaligned pointer accepted")
	}
	for offset := 0; offset < len(buffer); offset += 2 {
		binary.LittleEndian.PutUint16(buffer[offset:offset+2], 'A')
	}
	if _, err := installServiceString(buffer, (*uint16)(unsafe.Pointer(&buffer[4]))); err != ErrAuthority {
		t.Fatal("unterminated buffer accepted")
	}
}

func TestInstallSCMQueriesNativeServiceWithoutMutation(t *testing.T) {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		t.Fatal("read-only SCM open failed")
	}
	defer windows.CloseServiceHandle(manager)
	name, _ := windows.UTF16PtrFromString("EventLog")
	handle, err := windows.OpenService(manager, name, windows.READ_CONTROL|windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		t.Fatal("read-only native service open failed")
	}
	defer windows.CloseServiceHandle(handle)
	config, err := readInstallServiceConfig(handle)
	if err != nil || config.binary == "" {
		t.Fatal("native service config parser failed")
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_SERVICE, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal("native descriptor read failed")
	}
	_ = protectedInstallService(descriptor)
	buffer := make([]byte, 8192)
	var needed uint32
	if err := windows.QueryServiceConfig2(handle, windows.SERVICE_CONFIG_FAILURE_ACTIONS, &buffer[0], uint32(len(buffer)), &needed); err != nil {
		t.Fatal("native failure-action query failed")
	}
	actions := (*windows.SERVICE_FAILURE_ACTIONS)(unsafe.Pointer(&buffer[0]))
	if _, err := installServiceString(buffer, actions.Command); err != nil {
		t.Fatal("native failure-action command parser failed")
	}
	if err := windows.QueryServiceConfig2(handle, windows.SERVICE_CONFIG_TRIGGER_INFO, &buffer[0], uint32(len(buffer)), &needed); err != nil {
		t.Fatal("native trigger query failed")
	}
}

func TestInstallBackendZeroValueCannotOpenOrControlSCM(t *testing.T) {
	ctx := context.Background()
	var backend windowsInstallBackend
	if _, err := backend.Lock(ctx); err != ErrAuthority {
		t.Fatal("zero backend acquired ownership")
	}
	if backend.Stop(ctx) != ErrAuthority || backend.Start(ctx) != ErrAuthority || backend.Replace(ctx, installImages{}) != ErrAuthority || backend.Finish(ctx) != ErrAuthority {
		t.Fatal("zero backend attempted privileged operation")
	}
	if _, err := backend.Snapshot(ctx); err != ErrAuthority {
		t.Fatal("zero backend created a snapshot")
	}
	if backend.Close() != nil || backend.Close() != nil {
		t.Fatal("zero backend close is not idempotent")
	}
}

func TestInstallPayloadAcceptsCleanBuildsAndOwnsItsBytes(t *testing.T) {
	directory := os.Getenv("KRASIS_INSTALL_FIXTURE_DIR")
	if directory == "" {
		t.Skip("clean committed binary fixtures are checked by the post-commit fixture intent")
	}
	agent, err := os.ReadFile(filepath.Join(directory, "krasis-agent.exe"))
	if err != nil {
		t.Fatal("agent fixture missing")
	}
	input, err := os.ReadFile(filepath.Join(directory, "krasis-input.exe"))
	if err != nil {
		t.Fatal("input fixture missing")
	}
	images := installImages{Agent: sha256.Sum256(agent), Service: sha256.Sum256(input)}
	payload, err := pinnedInstallPayload(agent, input, images)
	if err != nil || !payload.valid() {
		t.Fatal("clean revision-matched command images rejected")
	}
	if _, err := pinnedInstallPayload(input, agent, installImages{Agent: images.Service, Service: images.Agent}); err != ErrAuthority {
		t.Fatal("approved hashes permitted swapped command roles")
	}
	agent[0] ^= 0xff
	input[0] ^= 0xff
	if !payload.valid() {
		t.Fatal("caller changed pinned payload after authorization")
	}
}
