//go:build windows

package launchrecord

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func InJob() (bool, error) {
	var inJob int32
	result, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)))
	if result == 0 {
		return false, err
	}
	return inJob != 0, nil
}

// StartIndependent keeps ordinary launches cheap. Inside a terminal job, WMI
// owns the child creation; DETACHED_PROCESS alone does not break job membership.
// No shell source is assembled from paths or arguments. Callers validate images.
func StartIndependent(command *exec.Cmd) error {
	inJob, err := InJob()
	if err != nil {
		return err
	}
	if inJob {
		return startViaWMI(command)
	}
	if err = command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

const independentScript = `
$ErrorActionPreference='Stop'
try {
  $krasisStartup=([wmiclass]'Win32_ProcessStartup').CreateInstance()
  $krasisStartup.ShowWindow=0
  $krasisStartup.CreateFlags=16777736
  $krasisStartup.EnvironmentVariables=[string[]](ConvertFrom-Json $env:KRASIS_LAUNCH_ENVIRONMENT)
  $krasisStartup.WinstationDesktop='winsta0\default'
  $krasisResult=([wmiclass]'Win32_Process').Create($env:KRASIS_LAUNCH_COMMAND,$env:KRASIS_LAUNCH_DIRECTORY,$krasisStartup)
  @{code=[uint32]$krasisResult.ReturnValue;pid=[uint32]$krasisResult.ProcessId}|ConvertTo-Json -Compress
  if($krasisResult.ReturnValue -ne 0){exit 1}
} catch { exit 2 }
`

func startViaWMI(command *exec.Cmd) error {
	if command == nil || !filepath.IsAbs(command.Path) || !filepath.IsAbs(command.Dir) || len(command.Args) == 0 {
		return ErrInvalid
	}
	args := append([]string{command.Path}, command.Args[1:]...)
	for i, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return ErrInvalid
		}
		args[i] = syscall.EscapeArg(arg)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	units := utf16.Encode([]rune(independentScript))
	encoded := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], u)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	helper := exec.CommandContext(ctx, filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	helper.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	childEnv := append(windowsLaunchEnvironment(), "KRASIS_INDEPENDENT_LAUNCH=1")
	environment, _ := json.Marshal(childEnv)
	helper.Env = append(os.Environ(), "KRASIS_LAUNCH_COMMAND="+strings.Join(args, " "), "KRASIS_LAUNCH_DIRECTORY="+command.Dir, "KRASIS_LAUNCH_ENVIRONMENT="+string(environment))
	data, err := helper.Output()
	if err != nil {
		return errors.New("independent launch failed")
	}
	var result struct {
		Code uint32 `json:"code"`
		PID  uint32 `json:"pid"`
	}
	if json.Unmarshal(data, &result) != nil || result.Code != 0 || result.PID == 0 {
		return errors.New("independent launch unconfirmed")
	}
	return nil
}

// Ordinary and job-detached launches need the same Windows shell paths.
// Keep one allowlist so a launch mode cannot silently drop SystemDrive or
// ProgramData, or forward arbitrary caller values.
func windowsLaunchEnvironment() []string {
	var environment []string
	if os.Getenv("KRASIS_RUNTIME_TRACE") == "1" {
		environment = append(environment, "KRASIS_RUNTIME_TRACE=1")
	}
	for _, key := range []string{
		"SystemRoot", "SystemDrive", "WINDIR", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "USERPROFILE", "PATH",
		"ProgramData", "ALLUSERSPROFILE", "PUBLIC", "HOMEDRIVE", "HOMEPATH", "USERNAME", "USERDOMAIN", "USERDOMAIN_ROAMINGPROFILE",
		"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "CommonProgramFiles", "CommonProgramW6432", "CommonProgramFiles(x86)", "COMSPEC", "PATHEXT",
	} {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}
