//go:build windows

package localapi

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestUpgradeStopWaitsForKernelExitAfterAPIAndLeaseDisappear(t *testing.T) {
	for _, mode := range []string{"kernel_exit", "cancel_wait", "changed_instance"} {
		t.Run(mode, func(t *testing.T) { testUpgradeKernelExit(t, mode) })
	}
}

func testUpgradeKernelExit(t *testing.T, mode string) {
	path := filepath.Join(t.TempDir(), "owned-image.exe")
	if err := os.WriteFile(path, []byte("pinned executable stand-in"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRuntimeExitOwnerHelper$")
	cmd.Env = append(os.Environ(), "KRASIS_RUNTIME_EXIT_HELPER=1", "KRASIS_RUNTIME_EXIT_IMAGE="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	scanner := bufio.NewScanner(out)
	readLine := func() string {
		line := make(chan string, 1)
		go func() {
			if scanner.Scan() {
				line <- scanner.Text()
			} else {
				line <- ""
			}
		}()
		select {
		case value := <-line:
			return value
		case <-ctx.Done():
			t.Fatal("child fixture did not respond")
			return ""
		}
	}
	address := readLine()
	if address == "" {
		t.Fatal("child did not publish endpoint")
	}
	client := &Client{credentials: ClientCredentials{Endpoint: Endpoint{Address: address}, Token: strings.Repeat("a", 64)}, http: &http.Client{Timeout: time.Second}}
	done := make(chan error, 1)
	go func() {
		instance := strings.Repeat("b", 32)
		if mode == "changed_instance" {
			instance = strings.Repeat("d", 32)
		}
		_, err := client.StopForUpgrade(ctx, instance, 999, strings.Repeat("c", 40))
		done <- err
	}()
	if mode == "changed_instance" {
		if err := <-done; !errors.Is(err, ErrAuthentication) {
			t.Error("changed instance was not rejected before stop", err)
		}
		_, _ = fmt.Fprintln(in, "exit")
		_ = in.Close()
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		return
	}
	if readLine() != "released" {
		t.Fatal("child did not release API/singleton")
	}
	if err := os.Rename(path, path+".moved"); err == nil {
		t.Error("child no longer owns image fixture")
	}
	early := false
	if mode == "cancel_wait" {
		cancel()
	}
	select {
	case err := <-done:
		early = true
		if mode != "cancel_wait" || !errors.Is(err, context.Canceled) {
			t.Error("upgrade stop completed while exact native owner still held its image", err)
		}
	case <-time.After(60 * time.Millisecond):
		if mode == "cancel_wait" {
			t.Error("canceled exit wait did not return promptly")
		}
	}
	_, _ = fmt.Fprintln(in, "exit")
	_ = in.Close()
	if !early {
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal("kernel exit was never observed")
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal("exited owner retained image", err)
	}
}

func TestRuntimeExitOwnerHelper(t *testing.T) {
	if os.Getenv("KRASIS_RUNTIME_EXIT_HELPER") != "1" {
		return
	}
	name, _ := windows.UTF16PtrFromString(os.Getenv("KRASIS_RUNTIME_EXIT_IMAGE"))
	image, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		os.Exit(2)
	}
	mutexName, _ := windows.UTF16PtrFromString(fmt.Sprintf(`Local\Krasis-Exit-Fixture-%d`, os.Getpid()))
	mutex, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil {
		os.Exit(2)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		os.Exit(2)
	}
	stopped := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/v1/runtime" {
			_ = json.NewEncoder(w).Encode(RuntimeInfo{SchemaVersion: RuntimeSchemaVersion, Protocol: 1, Generation: 1, Build: "fixture", Instance: strings.Repeat("b", 32)})
			return
		}
		if r.URL.Path != "/v1/upgrade-stop" || r.Header.Get("If-Match") != strings.Repeat("b", 32) {
			w.WriteHeader(409)
			return
		}
		_ = json.NewEncoder(w).Encode(OperationResult{SchemaVersion: ResultSchemaVersion, Status: "COMPLETED", Code: "upgrade_stop_requested"})
		w.(http.Flusher).Flush()
		close(stopped)
	})}
	go server.Serve(listener)
	fmt.Fprintln(os.Stdout, listener.Addr().String())
	exit := make(chan struct{})
	go func() { _, _ = bufio.NewReader(os.Stdin).ReadString('\n'); close(exit) }()
	select {
	case <-stopped:
	case <-exit:
		_ = server.Close()
		_ = windows.CloseHandle(mutex)
		_ = windows.CloseHandle(image)
		os.Exit(0)
	}
	_ = server.Close()
	_ = windows.CloseHandle(mutex)
	fmt.Fprintln(os.Stdout, "released")
	<-exit
	_ = windows.CloseHandle(image)
	os.Exit(0)
}

func TestRuntimeListenerOwnershipRejectsMalformedOrAmbiguousTables(t *testing.T) {
	valid := make([]byte, 28)
	binary.LittleEndian.PutUint32(valid, 1)
	binary.LittleEndian.PutUint32(valid[4:], 2)
	copy(valid[8:12], []byte{127, 0, 0, 1})
	binary.BigEndian.PutUint16(valid[12:], 24789)
	binary.LittleEndian.PutUint32(valid[24:], 1234)
	if pid, err := runtimeListenerPID(valid, 24789); err != nil || pid != 1234 {
		t.Fatal(pid, err)
	}
	for _, mode := range []string{"truncated", "count", "wildcard", "port", "zero_pid", "duplicate"} {
		data := append([]byte(nil), valid...)
		switch mode {
		case "truncated":
			data = data[:27]
		case "count":
			binary.LittleEndian.PutUint32(data, ^uint32(0))
		case "wildcard":
			data[8] = 0
		case "port":
			data[12] ^= 1
		case "zero_pid":
			binary.LittleEndian.PutUint32(data[24:], 0)
		case "duplicate":
			data = append(data, valid[4:]...)
			binary.LittleEndian.PutUint32(data, 2)
		}
		if _, err := runtimeListenerPID(data, 24789); err == nil {
			t.Error("invalid owner table accepted", mode)
		}
	}
}
