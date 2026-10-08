//go:build windows

package appsettings

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
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

	"github.com/0disoft/krasis/internal/launchrecord"
)

// The task runs the remembered GUI image in background recovery mode as the
// limited logged-in user. The console CLI remains available for terminal use.
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
    $krasisAction=$krasisDefinition.Actions.Item(1)
    $krasisLegacy=($krasisDefinition.RegistrationInfo.Description -eq 'Krasis recovery v1' -and
      $krasisAction.Path -eq $env:KRASIS_RECOVERY_LEGACY_TARGET -and
      $krasisAction.Arguments -eq 'app ensure --json')
    $krasisPrevious=[string[]](ConvertFrom-Json $env:KRASIS_RECOVERY_PREVIOUS)
    $krasisOwned=($krasisDefinition.RegistrationInfo.Description -eq 'Krasis recovery v2' -and
      $krasisAction.Arguments -eq '--background-recovery' -and
      ($krasisAction.Path -eq $env:KRASIS_RECOVERY_TARGET -or $krasisPrevious -contains $krasisAction.Path))
    if ((-not $krasisLegacy -and -not $krasisOwned) -or
        $krasisTaskUser -ne $krasisSid -or
        $krasisDefinition.Principal.LogonType -ne 3 -or
        $krasisDefinition.Principal.RunLevel -ne 0 -or
        $krasisDefinition.Triggers.Count -ne 1 -or
        $krasisDefinition.Triggers.Item(1).Type -ne 1 -or
        $krasisDefinition.Triggers.Item(1).Repetition.Interval -ne 'PT1M' -or
        $krasisDefinition.Settings.ExecutionTimeLimit -ne 'PT30S' -or
        $krasisDefinition.Actions.Count -ne 1) { exit 3 }
    if ($env:KRASIS_RECOVERY_CHECK -eq '1') {
      if ($krasisTask.Enabled -and $krasisOwned -and $krasisAction.Path -eq $env:KRASIS_RECOVERY_TARGET) { exit 0 } else { exit 4 }
    }
    if ($env:KRASIS_RECOVERY_ENABLED -eq '0') { $krasisFolder.DeleteTask($krasisName,0);exit 0 }
    if (-not (Test-Path -LiteralPath $env:KRASIS_RECOVERY_TARGET -PathType Leaf)) { exit 2 }
    $krasisDefinition.RegistrationInfo.Description='Krasis recovery v2'
    $krasisDefinition.Settings.Enabled=$true
    $krasisAction.Path=$env:KRASIS_RECOVERY_TARGET
    $krasisAction.Arguments='--background-recovery'
    $krasisAction.WorkingDirectory=[IO.Path]::GetDirectoryName($env:KRASIS_RECOVERY_TARGET)
    $krasisSddl='D:P(A;;GA;;;SY)(A;;GA;;;'+$krasisSid+')'
    $null=$krasisFolder.RegisterTaskDefinition($krasisName,$krasisDefinition,6,$krasisSid,$null,3,$krasisSddl)
    exit 0
  }
  if ($env:KRASIS_RECOVERY_CHECK -eq '1') { exit 4 }
  if ($env:KRASIS_RECOVERY_ENABLED -eq '0') { exit 0 }
  if (-not (Test-Path -LiteralPath $env:KRASIS_RECOVERY_TARGET -PathType Leaf)) { exit 2 }
  $krasisDefinition=$krasisService.NewTask(0)
  $krasisDefinition.RegistrationInfo.Description='Krasis recovery v2'
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
  $krasisAction.Arguments='--background-recovery'
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

// Setup checks ownership before stopping a usable app. Missing, disabled and
// owned stale tasks are repairable; an unreadable or foreign task is not.
func ValidateRecoveryTaskRegistration(base string) error {
	_, err := RecoveryTaskEnabled(base)
	return err
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
	record, err := launchrecord.Load(base)
	if err != nil && !(inspect && errors.Is(err, launchrecord.ErrMissing)) {
		return errors.New("recovery launch record unavailable")
	}
	if enabled && !inspect {
		if _, err = launchrecord.Inspect(base); err != nil {
			return errors.New("recovery launch image invalid")
		}
	}
	previous, err := launchrecord.RecoveryDirectories(base)
	if err != nil {
		return errors.New("recovery launch history unavailable")
	}
	for i := range previous {
		previous[i] = filepath.Join(previous[i], "krasis.exe")
	}
	history, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := recoveryScriptCommand(ctx, system, recoveryTaskScript)
	flag := "0"
	if enabled {
		flag = "1"
	}
	check := "0"
	if inspect {
		check = "1"
	}
	cmd.Env = append(os.Environ(), "KRASIS_RECOVERY_TARGET="+record.Executable, "KRASIS_RECOVERY_LEGACY_TARGET="+filepath.Join(base, "Krasis", "launch", "krasis-cli.exe"), "KRASIS_RECOVERY_PREVIOUS="+string(history), "KRASIS_RECOVERY_ENABLED="+flag, "KRASIS_RECOVERY_SUFFIX="+suffix, "KRASIS_RECOVERY_CHECK="+check)
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

func recoveryScriptCommand(ctx context.Context, system, script string) *exec.Cmd {
	units := utf16.Encode([]rune(script))
	encoded := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], u)
	}
	cmd := exec.CommandContext(ctx, filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}
