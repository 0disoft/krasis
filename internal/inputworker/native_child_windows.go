//go:build windows

package inputworker

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
	"golang.org/x/sys/windows"
)

const nativeBootstrapSize = bootstrapSize + 16

var nativeHandleInformation = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")

// RunInstalledNativeInput accepts only the fixed protected worker mode. The
// inherited service/agent pipe handles are identity proofs, never read/write
// transports here. Both authenticators run independently before native input.
func RunInstalledNativeInput(ctx context.Context, installation *Installation) error {
	return runInstalledNativeInput(ctx, installation, false)
}

func RunInstalledNativeRelease(ctx context.Context, installation *Installation) error {
	return runInstalledNativeInput(ctx, installation, true)
}

func runInstalledNativeInput(ctx context.Context, installation *Installation, releaseOnly bool) error {
	agentDigest, serviceDigest, err := installation.workerDigests()
	if err != nil || !fixedInstalledInputProcess(serviceDigest, true, installation) {
		return ErrAuthority
	}
	var data [nativeBootstrapSize]byte
	if _, err := io.ReadFull(os.Stdin, data[:]); err != nil {
		return ErrAuthority
	}
	var packet [bootstrapSize]byte
	copy(packet[:], data[:bootstrapSize])
	agent, image, err := decodeWorkerBootstrapHandles(packet, agentDigest)
	if err != nil {
		return ErrAuthority
	}
	values := []uint64{agent, image, binary.BigEndian.Uint64(data[bootstrapSize : bootstrapSize+8]), binary.BigEndian.Uint64(data[bootstrapSize+8:])}
	handles, err := adoptNativeHandles(values)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handles[0])
	defer windows.CloseHandle(handles[2])
	defer windows.CloseHandle(handles[3])
	imageFile := os.NewFile(uintptr(handles[1]), "native-input-agent-image")
	defer func() {
		if imageFile != nil {
			_ = imageFile.Close()
		}
	}()
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	proof, err := authenticateServiceInstalled(startup, uintptr(handles[2]), serviceDigest, installation)
	if err != nil {
		return err
	}
	defer proof.Close()
	var recoveryService windows.Handle
	if releaseOnly {
		recoveryService, err = pinNativeRecoveryService(handles[2], proof)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(recoveryService)
	}
	if installation.adoptWorkerAgentImage(proof, imageFile) != nil {
		return ErrAuthority
	}
	imageFile = nil
	authority, err := authenticateAgentInstalled(uintptr(handles[0]), agentDigest, installation)
	if err != nil {
		return err
	}
	defer authority.Close()
	if !validNativeParent(handles[3], installation, serviceDigest) {
		return ErrAuthority
	}
	var leaseData [leaseSize]byte
	copy(leaseData[:], packet[bootstrapLeaseOffset:])
	lease, err := decodeLease(leaseData, time.Now())
	if err != nil || lease.Capture || lease.Presentation || startup.Err() != nil || wininput.InitializePhysicalCoordinates() != nil {
		return ErrAuthority
	}
	input := wininput.NewInjectionPort()
	serviceCheck := func() error { return proof.check(false) }
	if releaseOnly {
		serviceCheck = func() error { return checkNativeRecoveryService(recoveryService, proof) }
	}
	return inputdesktop.RunInteractive(ctx, func() error { return authority.check(releaseOnly) }, func(scope *inputdesktop.Scope) error {
		execute := func(ctx context.Context, work func() error) error {
			if (!releaseOnly && !time.Now().Before(lease.ExpiresAt)) || authority.check(releaseOnly) != nil || serviceCheck() != nil || !nativeParentAlive(handles[3]) {
				return ErrAuthority
			}
			return scope.Do(ctx, work)
		}
		if releaseOnly {
			// Guard-side commands have no native release capability. Only the
			// release-only framed handler executes within fresh authority/scope.
			_, err := crashguard.ServeWithInput(os.Stdin, os.Stdout, deniedGuardRelease{}, PortableReleaseHandler(input, execute))
			return err
		}
		// The parent worker owns the authoritative journal. This child's guard
		// journal stays empty; retirement must precede the parent's key-up.
		_, err := crashguard.ServeWithInput(os.Stdin, os.Stdout, input, PortableInputHandler(input, execute))
		return err
	})
}

// Pin the exact fully authenticated service before input admission. Cleanup
// tolerates SCM StopPending, but never another/reused service process or pipe.
func pinNativeRecoveryService(pipe windows.Handle, proof *Authority) (windows.Handle, error) {
	if proof == nil || proof.kind != authorityService || proof.check(false) != nil {
		return 0, ErrAuthority
	}
	var pid uint32
	if windows.GetNamedPipeServerProcessId(pipe, &pid) != nil {
		return 0, ErrAuthority
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return 0, ErrAuthority
	}
	if !nativeParentAlive(handle) || proof.check(false) != nil {
		_ = windows.CloseHandle(handle)
		return 0, ErrAuthority
	}
	return handle, nil
}

func checkNativeRecoveryService(handle windows.Handle, proof *Authority) error {
	if proof == nil || proof.kind != authorityService || !nativeParentAlive(handle) || proof.checkHeartbeat() != nil {
		return ErrAuthority
	}
	return nil
}

func adoptNativeHandles(values []uint64) ([]windows.Handle, error) {
	return adoptNativeReferences(values, 4)
}

func adoptNativeReferences(values []uint64, expected int) ([]windows.Handle, error) {
	if len(values) != expected {
		return nil, ErrAuthority
	}
	handles := make([]windows.Handle, 0, len(values))
	for _, value := range values {
		if value == 0 || uint64(uintptr(value)) != value {
			return nil, ErrAuthority
		}
		handle := windows.Handle(value)
		for _, prior := range handles {
			if prior == handle {
				return nil, ErrAuthority
			}
		}
		var flags uint32
		ok, _, _ := nativeHandleInformation.Call(uintptr(handle), uintptr(unsafe.Pointer(&flags)))
		if ok == 0 || flags&windows.HANDLE_FLAG_INHERIT == 0 {
			return nil, ErrAuthority
		}
		handles = append(handles, handle)
	}
	for _, handle := range handles {
		if windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, 0) != nil {
			for _, owned := range handles {
				_ = windows.CloseHandle(owned)
			}
			return nil, ErrAuthority
		}
	}
	return handles, nil
}

func nativeParentAlive(handle windows.Handle) bool {
	state, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
}

func validNativeParent(handle windows.Handle, installation *Installation, digest [32]byte) bool {
	pid, err := windows.GetProcessId(handle)
	if err != nil || pid == windows.GetCurrentProcessId() || !nativeParentAlive(handle) {
		return false
	}
	var token windows.Token
	if windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token) != nil {
		return false
	}
	defer token.Close()
	if !validConsoleToken(token, windows.WTSGetActiveConsoleSessionId()) {
		return false
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return false
	}
	path := filepath.Join(root, "Krasis", "Input", "krasis-input.exe")
	var image [32768]uint16
	size := uint32(len(image))
	if windows.QueryFullProcessImageName(handle, 0, &image[0], &size) != nil || size == 0 || size >= uint32(len(image)) || !strings.EqualFold(path, windows.UTF16ToString(image[:size])) {
		return false
	}
	pin, err := pinIdentityImage(installation, path, digest)
	if err != nil {
		return false
	}
	return pin.Close() == nil
}

func startIsolatedInput(ctx context.Context, control *pipeConn, channel *Channel, proof *Authority, lease Lease, installation *Installation) (*isolatedInput, error) {
	agentDigest, serviceDigest, err := installation.digests()
	if err != nil || control == nil || channel == nil || channel.authority == nil || proof == nil || proof.kind != authorityService || proof.check(false) != nil || channel.authority.check(false) != nil || lease.Capture || lease.Presentation || !fixedInstalledInputProcess(serviceDigest, true, installation) {
		return nil, ErrAuthority
	}
	agentPipe, ok := channel.stream.(*pipeConn)
	if !ok {
		return nil, ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	image, err := pinIdentityImage(installation, filepath.Join(root, "Krasis", "Input", "krasis-agent.exe"), agentDigest)
	if err != nil {
		return nil, err
	}
	defer image.Close()
	var handles []windows.Handle
	defer func() {
		for _, handle := range handles {
			_ = windows.CloseHandle(handle)
		}
	}()
	for _, source := range []windows.Handle{agentPipe.handle, windows.Handle(image.Fd()), control.handle, windows.CurrentProcess()} {
		var handle windows.Handle
		access, flags := uint32(0), uint32(windows.DUPLICATE_SAME_ACCESS)
		if source == windows.CurrentProcess() {
			access, flags = windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, 0
		}
		if windows.DuplicateHandle(windows.CurrentProcess(), source, windows.CurrentProcess(), &handle, access, true, flags) != nil {
			return nil, ErrAuthority
		}
		handles = append(handles, handle)
	}
	leaseData, err := encodeLease(lease, time.Now())
	if err != nil {
		return nil, err
	}
	packet, err := encodeWorkerBootstrap(uint64(handles[0]), uint64(handles[1]), agentDigest, leaseData)
	if err != nil {
		return nil, err
	}
	var data [nativeBootstrapSize]byte
	copy(data[:], packet[:])
	binary.BigEndian.PutUint64(data[bootstrapSize:bootstrapSize+8], uint64(handles[2]))
	binary.BigEndian.PutUint64(data[bootstrapSize+8:], uint64(handles[3]))
	return startNativePair(ctx, func(run context.Context, release bool) (*isolatedInput, error) {
		mode := "--native-input-worker"
		if release {
			mode = "--native-release-worker"
		}
		return startNativeChildClient(run, mode, handles, data[:])
	})
}

// Framing/retirement is shared; the two callers retain distinct authenticators
// and immutable desktop policies. No caller-selected command is admitted.
func startNativeChildClient(ctx context.Context, mode string, handles []windows.Handle, data []byte) (*isolatedInput, error) {
	if ctx == nil || ctx.Err() != nil || (mode != "--native-input-worker" && mode != "--native-login-worker" && mode != "--native-release-worker" && mode != "--native-login-release-worker") {
		return nil, ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	command := exec.Command(filepath.Join(root, "Krasis", "Input", "krasis-input.exe"), mode)
	command.Dir = filepath.Dir(command.Path)
	command.Env = []string{} // No inherited PATH, profile, secrets or selectors.
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	for _, handle := range handles {
		command.SysProcAttr.AdditionalInheritedHandles = append(command.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(handle))
	}
	process, err := crashguard.StartLocalInputCommand(command, data)
	if err != nil {
		var pending *crashguard.ExitPending
		if errors.As(err, &pending) {
			return nil, errors.Join(err, ErrRecovery)
		}
		return nil, err
	}
	client, err := newPortableClient(ctx, process)
	if err != nil {
		closeErr := process.Close()
		var pending *crashguard.ExitPending
		if errors.As(closeErr, &pending) {
			return nil, errors.Join(err, closeErr, ErrRecovery)
		}
		return nil, errors.Join(err, closeErr)
	}
	return newIsolatedInput(client, process, wininput.NewInjectionPort()), nil
}
