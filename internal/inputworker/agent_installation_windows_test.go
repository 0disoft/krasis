//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"os"
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

func TestAgentServiceRejectsUnboundClosedAndMismatchedInstallation(t *testing.T) {
	ctx := context.Background()
	var empty *AgentService
	if empty.Close() != nil {
		t.Fatal("nil owner cleanup failed")
	}
	installation := imagePinFixture(t)
	installation.agentDigest, installation.serviceDigest = installation.images[0].digest, installation.images[1].digest
	mismatch := installation.serviceDigest
	mismatch[0] ^= 1
	for _, owner := range []*AgentService{nil, {}, {installation: installation}, {installation: installation, digest: mismatch}} {
		if client, err := owner.ConnectLease(ctx, testLease()); client != nil || err != ErrAuthority {
			t.Fatal("unbound installation opened a service channel")
		}
	}
	owner := &AgentService{installation: installation, digest: installation.serviceDigest}
	if owner.Close() != nil || owner.Close() != nil {
		t.Fatal("owner cleanup not idempotent")
	}
	if client, err := owner.ConnectLease(ctx, testLease()); client != nil || err != ErrAuthority {
		t.Fatal("closed installation reopened service access")
	}
	if err := os.WriteFile(installation.images[1].path, []byte("replacement"), 0600); err != nil {
		t.Fatal("connection cleanup retained an image pin", err)
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
