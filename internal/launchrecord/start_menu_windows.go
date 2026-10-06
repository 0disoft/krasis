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

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
)

// The shell shortcut targets the fixed recovery CLI, not a generation folder.
// The bounded helper runs unelevated and never invokes the shortcut or the app.
func RegisterStartMenu(localAppData string) error {
	if _, err := Inspect(localAppData); err != nil {
		return err
	}
	dir, err := Directory(localAppData)
	if err != nil {
		return err
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return err
	}
	guard, err := productidentity.PrepareLaunch(localAppData)
	if err != nil {
		return err
	}
	defer guard.Close()
	return ensureStartMenuLink(guard, filepath.Join(dir, "krasis-cli.exe"), filepath.Join(programs, "Krasis.lnk"), registerStartMenuLink)
}

type menuReceipt struct{ Target, Link, SHA256 string }

func linkDigest(link string) (string, error) {
	dir, err := securefs.OpenReadDirectory(filepath.Dir(link))
	if err != nil {
		return "", err
	}
	defer dir.Close()
	f, err := dir.Open(filepath.Base(link))
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() > 65536 {
		return "", ErrInvalid
	}
	return digest(f)
}

// A protected receipt skips the COM helper only while the exact verified link
// bytes and fixed target remain unchanged. A removed/edited link is rechecked.
func ensureStartMenuLink(dir *securefs.Directory, target, link string, register func(string, string) error) error {
	if f, err := dir.Open("start-menu.json"); err == nil {
		var r menuReceipt
		info, statErr := f.Stat()
		if statErr == nil && info.Size() <= 8192 {
			_ = json.NewDecoder(f).Decode(&r)
		}
		f.Close()
		if r.Target == target && r.Link == link && r.SHA256 != "" {
			if hash, err := linkDigest(link); err == nil && hash == r.SHA256 {
				return nil
			}
		}
	}
	if err := register(target, link); err != nil {
		return err
	}
	hash, err := linkDigest(link)
	if err != nil {
		return err
	}
	data, err := json.Marshal(menuReceipt{target, link, hash})
	if err != nil {
		return err
	}
	return replace(dir, "start-menu.json", data)
}

const startMenuScript = `
$ErrorActionPreference = 'Stop'
try {
  $krasisTarget = $env:KRASIS_MENU_TARGET
  $krasisLink = $env:KRASIS_MENU_LINK
  if (-not (Test-Path -LiteralPath $krasisTarget -PathType Leaf)) { exit 2 }
  $krasisShell = New-Object -ComObject WScript.Shell
  if (Test-Path -LiteralPath $krasisLink) {
    $krasisItem = Get-Item -LiteralPath $krasisLink -Force
    if ($krasisItem.Attributes -band [IO.FileAttributes]::ReparsePoint) { exit 3 }
    $krasisExisting = $krasisShell.CreateShortcut($krasisLink)
    if ($krasisExisting.TargetPath -ne $krasisTarget -or $krasisExisting.Arguments -ne 'relaunch') { exit 3 }
    exit 0
  }
  $krasisShortcut = $krasisShell.CreateShortcut($krasisLink)
  $krasisShortcut.TargetPath = $krasisTarget
  $krasisShortcut.Arguments = 'relaunch'
  $krasisShortcut.WorkingDirectory = [IO.Path]::GetDirectoryName($krasisTarget)
  $krasisShortcut.IconLocation = $krasisTarget + ',0'
  $krasisShortcut.Description = 'Krasis'
  $krasisShortcut.Save()
  $krasisSaved = $krasisShell.CreateShortcut($krasisLink)
  if ($krasisSaved.TargetPath -ne $krasisTarget -or $krasisSaved.Arguments -ne 'relaunch') { exit 4 }
  exit 0
} catch { exit 1 }
`

func registerStartMenuLink(target, link string) error {
	if !filepath.IsAbs(target) || !filepath.IsAbs(link) || filepath.Base(target) != "krasis-cli.exe" ||
		filepath.Base(link) != "Krasis.lnk" || strings.ContainsAny(target+link, "\x00\r\n\"") {
		return errors.New("invalid start menu paths")
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	units := utf16.Encode([]rune(startMenuScript))
	encoded := make([]byte, 2*len(units))
	for i, value := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"),
		"-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	command.Env = append(os.Environ(), "KRASIS_MENU_TARGET="+target, "KRASIS_MENU_LINK="+link)
	if err := command.Run(); err != nil {
		return errors.New("start menu registration failed")
	}
	info, err := os.Lstat(link)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 65536 {
		return errors.New("invalid start menu shortcut")
	}
	return nil
}
