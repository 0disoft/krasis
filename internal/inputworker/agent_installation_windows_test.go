//go:build windows

package inputworker

import (
	"crypto/sha256"
	"testing"

	"golang.org/x/sys/windows"
)

func TestAgentLaunchRejectsMissingDigest(t *testing.T) {
	launch, err := OpenAgentLaunch([sha256.Size]byte{})
	if launch != nil || err != ErrAuthority {
		t.Fatal("unbound installed agent accepted")
	}
	var empty *AgentLaunch
	if empty.Path() != "" || empty.Close() != nil {
		t.Fatal("zero launch exposed executable")
	}
}

func TestPortableTestProcessCannotEnableInstalledInput(t *testing.T) {
	digest, err := CurrentAgentServiceDigest()
	if digest != ([sha256.Size]byte{}) || err != ErrAuthority {
		t.Fatal("portable test process enabled privileged input")
	}
}

func TestStoppedOrPartialInputServiceCannotSelectInstalledAgent(t *testing.T) {
	digest := installTestImages("installed").Service
	status := windows.SERVICE_STATUS_PROCESS{ServiceType: windows.SERVICE_WIN32_OWN_PROCESS, CurrentState: windows.SERVICE_RUNNING, ProcessId: 42}
	record := serviceRecord{Phase: serviceReady, PID: 42, Digest: digest}
	record.Run[0] = 1
	if !readyInstalledServiceStatus(status, record, digest) {
		t.Fatal("verified ready service rejected")
	}
	for _, state := range []uint32{windows.SERVICE_STOPPED, windows.SERVICE_START_PENDING, windows.SERVICE_STOP_PENDING} {
		copy := status
		copy.CurrentState, copy.Win32ExitCode = state, uint32(windows.ERROR_SERVICE_NEVER_STARTED)
		if readyInstalledServiceStatus(copy, record, digest) {
			t.Fatal("non-ready service selected installed agent")
		}
	}
	for _, mutate := range []func(*serviceRecord){
		func(r *serviceRecord) { r.Phase = serviceStarting },
		func(r *serviceRecord) { r.Phase = serviceFailed },
		func(r *serviceRecord) { r.Phase = serviceClean },
		func(r *serviceRecord) { r.PID++ },
		func(r *serviceRecord) { r.Digest[0] ^= 1 },
		func(r *serviceRecord) { r.Run = Binding{} },
	} {
		copy := record
		mutate(&copy)
		if readyInstalledServiceStatus(status, copy, digest) {
			t.Fatal("stale/incomplete service receipt selected installed agent")
		}
	}
	status.ProcessId = 0
	if readyInstalledServiceStatus(status, record, digest) {
		t.Fatal("service without process selected installed agent")
	}
}
