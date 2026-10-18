//go:build windows

package launchrecord

import (
	"encoding/json"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestIndependentProcessChild(t *testing.T) {
	args := os.Args
	if len(args) < 3 || args[len(args)-2] != "--" {
		t.Skip("child entrypoint")
	}
	inJob, err := InJob()
	if err != nil {
		t.Fatal(err)
	}
	var session uint32
	if err = windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(struct {
		InJob       bool
		Session     uint32
		PID         uint32
		Independent bool
		SystemDrive string
		ProgramData string
		CallerValue string
	}{inJob, session, windows.GetCurrentProcessId(), os.Getenv("KRASIS_INDEPENDENT_LAUNCH") == "1", os.Getenv("SystemDrive"), os.Getenv("ProgramData"), os.Getenv("KRASIS_TEST_CALLER_VALUE")})
	if err = os.WriteFile(args[len(args)-1], data, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(args[len(args)-1] + ".done"); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestOrdinaryLaunchRetainsWindowsShellPathsWithoutCallerValues(t *testing.T) {
	t.Setenv("KRASIS_TEST_CALLER_VALUE", "synthetic caller value")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "ordinary launch.json")
	if err := os.WriteFile(result+".done", []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}
	// Execute the exact command constructed by Start/StartAtLogin without the
	// WMI detachment branch, which previously rebuilt the environment itself.
	command := launchCommand(self, "-test.run=^TestIndependentProcessChild$", "--", result)
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		SystemDrive string
		ProgramData string
		CallerValue string
	}
	if json.Unmarshal(data, &got) != nil {
		t.Fatal("ordinary child result malformed")
	}
	if got.SystemDrive == "" || !strings.EqualFold(got.SystemDrive, os.Getenv("SystemDrive")) ||
		!filepath.IsAbs(got.ProgramData) || !strings.EqualFold(got.ProgramData, os.Getenv("ProgramData")) {
		t.Fatal("Windows shell paths cannot resolve in ordinary child", got)
	}
	if got.CallerValue != "" {
		t.Fatal("ordinary launch forwarded unrelated caller state")
	}
}

func TestRuntimeTraceEnvironmentRequiresExactOptIn(t *testing.T) {
	for _, value := range []string{"", "true", "1"} {
		t.Setenv("KRASIS_RUNTIME_TRACE", value)
		found := false
		for _, entry := range windowsLaunchEnvironment() {
			if entry == "KRASIS_RUNTIME_TRACE=1" {
				found = true
			}
		}
		if found != (value == "1") {
			t.Fatalf("opt-in %q forwarded=%v", value, found)
		}
	}
}

func TestWMIChildIsOutsideCallerJobAndInCallerSession(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "launch result.json")
	t.Cleanup(func() { _ = os.WriteFile(result+".done", []byte("done"), 0600) })
	command := exec.Command(self, "-test.run=^TestIndependentProcessChild$", "--", result)
	command.Dir = filepath.Dir(self)
	if err = startViaWMI(command); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(result); err == nil {
			var got struct {
				InJob       bool
				Session     uint32
				PID         uint32
				Independent bool
				SystemDrive string
				ProgramData string
			}
			if json.Unmarshal(data, &got) != nil {
				t.Fatal("child result malformed")
			}
			var current uint32
			if err = windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &current); err != nil {
				t.Fatal(err)
			}
			if got.Session != current || !got.Independent {
				t.Fatal("child lifetime/session not independent", got, current)
			}
			if got.SystemDrive == "" || !strings.EqualFold(got.SystemDrive, os.Getenv("SystemDrive")) ||
				!filepath.IsAbs(got.ProgramData) || !strings.EqualFold(got.ProgramData, os.Getenv("ProgramData")) {
				t.Fatal("Windows shell paths cannot resolve in independent child")
			}
			inJob, _ := InJob()
			if inJob {
				var list struct {
					Assigned uint32
					Count    uint32
					IDs      [32768]uintptr
				}
				if err := windows.QueryInformationJobObject(0, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), nil); err != nil {
					t.Fatal(err)
				}
				if list.Count > uint32(len(list.IDs)) {
					t.Fatal("job list overflow")
				}
				for _, pid := range list.IDs[:list.Count] {
					if pid == uintptr(got.PID) {
						t.Fatal("child inherited caller's job")
					}
				}
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("independent child did not finish")
}
