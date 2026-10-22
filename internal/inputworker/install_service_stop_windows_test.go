//go:build windows

package inputworker

import (
	"context"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInstallerJoinsProcessAlreadyStoppingBeforeDirectoryReplacement(t *testing.T) {
	for _, initial := range []uint32{windows.SERVICE_RUNNING, windows.SERVICE_STOP_PENDING} {
		t.Run(map[uint32]string{windows.SERVICE_RUNNING: "running", windows.SERVICE_STOP_PENDING: "already_stopping"}[initial], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			state := initial
			pins, stops, cleanChecks, pauses := 0, 0, 0, 0
			var owner *processIdentity
			err := waitStoppedInstallService(ctx, installServiceStopOps{
				status: func() (windows.SERVICE_STATUS_PROCESS, error) {
					return windows.SERVICE_STATUS_PROCESS{CurrentState: state, ProcessId: windows.GetCurrentProcessId()}, nil
				},
				pin: func(status windows.SERVICE_STATUS_PROCESS) (*processIdentity, error) {
					pins++
					handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, status.ProcessId)
					if err != nil {
						t.Fatal(err)
					}
					owner = &processIdentity{handle: handle}
					return owner, nil
				},
				clean: func(_ windows.SERVICE_STATUS_PROCESS, pid uint32) error {
					cleanChecks++
					if pid != windows.GetCurrentProcessId() {
						t.Error("clean receipt was not bound to the retained process")
					}
					return nil
				},
				stop: func() error { stops++; return nil },
				pause: func(context.Context) error {
					pauses++
					state = windows.SERVICE_STOPPED
					if pauses == 2 {
						cancel() // SCM stopped, but the exact native process is still alive.
						return ctx.Err()
					}
					return nil
				},
			})
			wantStops := 0
			if initial == windows.SERVICE_RUNNING {
				wantStops = 1
			}
			if err != ErrRecovery || pins != 1 || stops != wantStops || cleanChecks != 1 || pauses != 2 {
				t.Fatal("SCM stop admitted replacement before process exit", err, pins, stops, cleanChecks, pauses)
			}
			if owner == nil || owner.handle != 0 {
				t.Fatal("retained stop-process handle leaked")
			}
		})
	}
}

func TestRetiringInstallProcessRequiresLiveVerificationAndAcceptsExactExit(t *testing.T) {
	if _, err := pinRetiringInstallProcess(windows.GetCurrentProcessId(), func() (*processIdentity, error) { return nil, ErrAuthority }); err != ErrAuthority {
		t.Fatal("live process skipped independent identity verification", err)
	}
	child := exec.Command("cmd.exe", "/c", "exit", "0")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	pid := uint32(child.Process.Pid)
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle) // Retain the exited process object/PID.
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	owner, err := pinRetiringInstallProcess(pid, func() (*processIdentity, error) {
		t.Fatal("exited process required live token/image inspection")
		return nil, ErrAuthority
	})
	if err != nil || owner != nil {
		t.Fatal("kernel-confirmed exit rejected", owner, err)
	}
}

func TestInstallerRejectsUnverifiedAlreadyStoppingProcess(t *testing.T) {
	err := waitStoppedInstallService(context.Background(), installServiceStopOps{
		status: func() (windows.SERVICE_STATUS_PROCESS, error) {
			return windows.SERVICE_STATUS_PROCESS{CurrentState: windows.SERVICE_STOP_PENDING}, nil
		},
		pin: func(windows.SERVICE_STATUS_PROCESS) (*processIdentity, error) { return nil, ErrAuthority },
		clean: func(windows.SERVICE_STATUS_PROCESS, uint32) error {
			t.Fatal("unverified process reached receipt check")
			return nil
		},
		stop:  func() error { t.Fatal("unverified process reached control"); return nil },
		pause: func(context.Context) error { t.Fatal("unverified process was accepted"); return nil },
	})
	if err != ErrAuthority {
		t.Fatal("unverified stopping service admitted", err)
	}
}
