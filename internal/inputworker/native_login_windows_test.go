//go:build windows

package inputworker

import (
	"context"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNativeLoginModeRequiresProtectedInstallationAndControlOwner(t *testing.T) {
	if RunInstalledNativeLoginInput(context.Background(), nil) == nil {
		t.Fatal("uninstalled native login mode admitted")
	}
	for _, owner := range []*loginNativeOwner{nil, {}, {proof: &Authority{kind: authorityAgent}}} {
		if _, err := owner.start(context.Background()); err == nil {
			t.Fatal("missing native login owner admitted")
		}
	}
}

func TestLoginJobKeepsBothConsoleAndNativeInputUnderOneLimit(t *testing.T) {
	job, err := makeConsoleWorkerJob(loginConsole)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(job)
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		t.Fatal(err)
	}
	if limits.BasicLimitInformation.ActiveProcessLimit != 2 || limits.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Fatal("login child escaped bounded kill-on-close group", limits.BasicLimitInformation)
	}
}

func TestOrdinaryRecoveryJobHasExactlyThreeProcessCapacity(t *testing.T) {
	if RunInstalledNativeRelease(context.Background(), nil) == nil {
		t.Fatal("uninstalled process admitted native recovery")
	}
	job, err := makeConsoleWorkerJob(ordinaryConsole)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(job)
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		t.Fatal(err)
	}
	if limits.BasicLimitInformation.ActiveProcessLimit != 3 || limits.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Fatal(limits.BasicLimitInformation)
	}
}
