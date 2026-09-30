//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestPrivilegeEnablementRequiresActualBoundedTokenEntry(t *testing.T) {
	luid := windows.LUID{LowPart: 7, HighPart: 2}
	var buffer [16]byte
	binary.LittleEndian.PutUint32(buffer[:4], 1)
	binary.LittleEndian.PutUint32(buffer[4:8], luid.LowPart)
	binary.LittleEndian.PutUint32(buffer[8:12], uint32(luid.HighPart))
	if containsEnabledPrivilege(buffer[:], luid) {
		t.Fatal("disabled privilege accepted")
	}
	binary.LittleEndian.PutUint32(buffer[12:], windows.SE_PRIVILEGE_ENABLED)
	if !containsEnabledPrivilege(buffer[:], luid) {
		t.Fatal("enabled privilege not observed")
	}
	if containsEnabledPrivilege(buffer[:15], luid) || containsEnabledPrivilege(buffer[:], windows.LUID{LowPart: 8}) {
		t.Fatal("unassigned or malformed privilege accepted")
	}
	binary.LittleEndian.PutUint32(buffer[:4], 0xffffffff)
	if containsEnabledPrivilege(buffer[:], luid) {
		t.Fatal("oversized token count accepted")
	}
}

func TestConsoleLaunchRejectsMissingAndServiceAuthority(t *testing.T) {
	var digest [sha256.Size]byte
	digest[0] = 1
	service := &Authority{kind: authorityService, checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	for _, authority := range []*Authority{nil, {}, service} {
		worker, err := LaunchConsoleWorker(context.Background(), authority, digest)
		if worker != nil || err != ErrAuthority {
			t.Fatal("untrusted launch authority accepted")
		}
	}
}

func TestConsoleLaunchRejectsOrdinaryProcessWithoutInput(t *testing.T) {
	self, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		t.Fatal("test identity unavailable")
	}
	system := self.system
	_ = self.close()
	if system {
		t.Skip("ordinary-process denial requires a non-SYSTEM test runner")
	}
	agent := &Authority{kind: authorityAgent, checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	var digest [sha256.Size]byte
	digest[0] = 1
	worker, err := LaunchConsoleWorker(context.Background(), agent, digest)
	if worker != nil || err != ErrAuthority {
		t.Fatal("ordinary process launched privileged child")
	}
}

func TestConsoleLaunchRejectsCancelledAndUnpinnedRequests(t *testing.T) {
	agent := &Authority{kind: authorityAgent, checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var digest [sha256.Size]byte
	digest[0] = 1
	for _, request := range []struct {
		ctx    context.Context
		digest [sha256.Size]byte
	}{{nil, digest}, {ctx, digest}, {context.Background(), [sha256.Size]byte{}}} {
		worker, err := LaunchConsoleWorker(request.ctx, agent, request.digest)
		if worker != nil || err != ErrAuthority {
			t.Fatal("invalid launch request accepted")
		}
	}
}

func TestNativeConsoleJobHasCrashAndChildProcessLimitsWithoutInput(t *testing.T) {
	job, err := makeWorkerJob()
	if err != nil {
		t.Fatal("owned job creation failed")
	}
	defer windows.CloseHandle(job)
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil) != nil {
		t.Fatal("job limits query failed")
	}
	want := uint32(windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS)
	if limits.BasicLimitInformation.LimitFlags != want || limits.BasicLimitInformation.ActiveProcessLimit != 1 {
		t.Fatal("unbounded or orphanable worker job")
	}
}

func TestConsoleCloseIsConcurrentAndZeroSafeWithoutInput(t *testing.T) {
	var zero ConsoleWorker
	if zero.Close() != nil || zero.Close() != nil {
		t.Fatal("zero worker close failed")
	}
	job, err := makeWorkerJob()
	if err != nil {
		t.Fatal("owned job creation failed")
	}
	worker := &ConsoleWorker{job: job, done: make(chan struct{})}
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			if worker.Close() != nil {
				t.Error("concurrent job close failed")
			}
		})
	}
	group.Wait()
	select {
	case <-worker.done:
	default:
		t.Fatal("worker close not signalled")
	}
}
