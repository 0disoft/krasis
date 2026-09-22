//go:build windows && desktop

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Set by the packager after building the shared UI. Never trust a writable
// manifest to authorize a different executable next to this launcher.
var controlSHA256 string

func verifyControl(file string, expected string) error {
	if len(expected) != 64 {
		return errors.New("missing packaged Workspace identity")
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return errors.New("Workspace checksum mismatch; extract the complete ZIP again")
	}
	return nil
}

func launch() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	control := filepath.Join(filepath.Dir(self), "peer-update", "krasis-control.exe")
	if err := verifyControl(control, controlSHA256); err != nil {
		return err
	}
	cmd := exec.Command(control, os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if len(os.Args) > 1 {
		return cmd.Run()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func main() {
	if err := launch(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if len(os.Args) == 1 {
			message, _ := syscall.UTF16PtrFromString("Cannot open Workspace. Extract the complete ZIP into a new folder.\n\n" + err.Error())
			title, _ := syscall.UTF16PtrFromString("Krasis")
			syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
		}
		os.Exit(1)
	}
}
