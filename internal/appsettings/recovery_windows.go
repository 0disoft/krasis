//go:build windows

package appsettings

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

// The task runs as the logged-in user with limited privileges. It polls only
// the fixed CLI; no network listener, password, arbitrary command or UAC bypass.
const recoveryTaskScript = `
$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
try {
  $krasisSid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
  $krasisName='Krasis Recovery '+$krasisSid+$env:KRASIS_RECOVERY_SUFFIX
  $krasisService=New-Object -ComObject Schedule.Service
  $krasisService.Connect()
  $krasisFolder=$krasisService.GetFolder('\')
  $krasisTask=$null
  try { $krasisTask=$krasisFolder.GetTask($krasisName) } catch {
    $krasisException=$_.Exception
    while ($krasisException.InnerException) { $krasisException=$krasisException.InnerException }
    if ($krasisException.HResult -ne -2147024894) { throw }
  }
  if ($krasisTask) {
    $krasisDefinition=$krasisTask.Definition
    $krasisTaskUser=$krasisDefinition.Principal.UserId
    if (-not $krasisTaskUser.StartsWith('S-')) {
      $krasisTaskUser=(New-Object Security.Principal.NTAccount($krasisTaskUser)).Translate([Security.Principal.SecurityIdentifier]).Value
    }
    if ($krasisDefinition.RegistrationInfo.Description -ne 'Krasis recovery v1' -or
        $krasisTaskUser -ne $krasisSid -or
        $krasisDefinition.Principal.LogonType -ne 3 -or
        $krasisDefinition.Principal.RunLevel -ne 0 -or
        $krasisDefinition.Triggers.Count -ne 1 -or
        $krasisDefinition.Triggers.Item(1).Type -ne 1 -or
        $krasisDefinition.Triggers.Item(1).Repetition.Interval -ne 'PT1M' -or
        $krasisDefinition.Settings.ExecutionTimeLimit -ne 'PT30S' -or
        $krasisDefinition.Actions.Count -ne 1 -or
        $krasisDefinition.Actions.Item(1).Path -ne $env:KRASIS_RECOVERY_TARGET -or
        $krasisDefinition.Actions.Item(1).Arguments -ne 'app ensure --json') { exit 3 }
    if ($env:KRASIS_RECOVERY_CHECK -eq '1') {
      if ($krasisTask.Enabled) { exit 0 } else { exit 4 }
    }
    if ($env:KRASIS_RECOVERY_ENABLED -eq '0') { $krasisFolder.DeleteTask($krasisName,0);exit 0 }
    if (-not $krasisTask.Enabled) { $krasisTask.Enabled=$true }
    exit 0
  }
  if ($env:KRASIS_RECOVERY_CHECK -eq '1') { exit 4 }
  if ($env:KRASIS_RECOVERY_ENABLED -eq '0') { exit 0 }
  if (-not (Test-Path -LiteralPath $env:KRASIS_RECOVERY_TARGET -PathType Leaf)) { exit 2 }
  $krasisDefinition=$krasisService.NewTask(0)
  $krasisDefinition.RegistrationInfo.Description='Krasis recovery v1'
  $krasisDefinition.Principal.UserId=$krasisSid
  $krasisDefinition.Principal.LogonType=3
  $krasisDefinition.Principal.RunLevel=0
  $krasisDefinition.Settings.Enabled=$true
  $krasisDefinition.Settings.DisallowStartIfOnBatteries=$false
  $krasisDefinition.Settings.StopIfGoingOnBatteries=$false
  $krasisDefinition.Settings.MultipleInstances=2
  $krasisDefinition.Settings.ExecutionTimeLimit='PT30S'
  $krasisTrigger=$krasisDefinition.Triggers.Create(1)
  $krasisTrigger.StartBoundary=(Get-Date).AddMinutes(1).ToString('s')
  $krasisTrigger.Repetition.Interval='PT1M'
  $krasisAction=$krasisDefinition.Actions.Create(0)
  $krasisAction.Path=$env:KRASIS_RECOVERY_TARGET
  $krasisAction.Arguments='app ensure --json'
  $krasisAction.WorkingDirectory=[IO.Path]::GetDirectoryName($env:KRASIS_RECOVERY_TARGET)
  $krasisSddl='D:P(A;;GA;;;SY)(A;;GA;;;'+$krasisSid+')'
  $null=$krasisFolder.RegisterTaskDefinition($krasisName,$krasisDefinition,2,$krasisSid,$null,3,$krasisSddl)
  exit 0
} catch {
  if ($env:KRASIS_RECOVERY_DEBUG -eq '1') {
    $krasisException=$_.Exception
    while ($krasisException.InnerException) { $krasisException=$krasisException.InnerException }
    Write-Output ('error='+$krasisException.HResult+';line='+$_.InvocationInfo.ScriptLineNumber)
  }
  exit 1
}
`

func SetRecoveryTask(base string, enabled bool) error {
	return setRecoveryTask(base, "", enabled)
}
func setRecoveryTask(base, suffix string, enabled bool) error {
	return runRecoveryTask(base, suffix, enabled, false)
}

var errRecoveryTaskInactive = errors.New("recovery task not active")

func RecoveryTaskEnabled(base string) (bool, error) {
	err := runRecoveryTask(base, "", false, true)
	if errors.Is(err, errRecoveryTaskInactive) {
		return false, nil
	}
	return err == nil, err
}

func runRecoveryTask(base, suffix string, enabled, inspect bool) error {
	if !filepath.IsAbs(base) || strings.ContainsAny(base, "\x00\r\n\"") {
		return errors.New("invalid recovery root")
	}
	if suffix != "" {
		if len(suffix) != 38 || !strings.HasPrefix(suffix, "-test-") {
			return errors.New("invalid recovery task suffix")
		}
		if _, err := hex.DecodeString(suffix[6:]); err != nil {
			return errors.New("invalid recovery task suffix")
		}
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	units := utf16.Encode([]rune(recoveryTaskScript))
	encoded := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], u)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	flag := "0"
	if enabled {
		flag = "1"
	}
	check := "0"
	if inspect {
		check = "1"
	}
	cmd.Env = append(os.Environ(), "KRASIS_RECOVERY_TARGET="+filepath.Join(base, "Krasis", "launch", "krasis-cli.exe"), "KRASIS_RECOVERY_ENABLED="+flag, "KRASIS_RECOVERY_SUFFIX="+suffix, "KRASIS_RECOVERY_CHECK="+check)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		var exitErr *exec.ExitError
		if inspect && errors.As(runErr, &exitErr) && exitErr.ExitCode() == 4 {
			return errRecoveryTaskInactive
		}
		if suffix != "" {
			return fmt.Errorf("recovery task registration failed: %v %s", runErr, strings.TrimSpace(string(output)))
		}
		return errors.New("recovery task registration failed")
	}
	return nil
}
