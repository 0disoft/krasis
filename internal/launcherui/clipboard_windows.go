//go:build windows

package launcherui

import (
	"context"
	"errors"
	"fmt"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"runtime"
	"syscall"
	unicode16 "unicode/utf16"
	"unsafe"
)

var clipUser = syscall.NewLazyDLL("user32.dll")
var clipKernel = syscall.NewLazyDLL("kernel32.dll")
var clipSequence = clipUser.NewProc("GetClipboardSequenceNumber")
var clipOpen = clipUser.NewProc("OpenClipboard")
var clipClose = clipUser.NewProc("CloseClipboard")
var clipGet = clipUser.NewProc("GetClipboardData")
var clipSet = clipUser.NewProc("SetClipboardData")
var clipEmpty = clipUser.NewProc("EmptyClipboard")
var clipAlloc = clipKernel.NewProc("GlobalAlloc")
var clipFree = clipKernel.NewProc("GlobalFree")
var clipLock = clipKernel.NewProc("GlobalLock")
var clipUnlock = clipKernel.NewProc("GlobalUnlock")
var clipSize = clipKernel.NewProc("GlobalSize")
var clipCopy = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")
var errClipboardUnavailable = errors.New("clipboard unavailable")
var errClipboardSize = errors.New("clipboard size limit")

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procRtlMoveMemory    = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")
)

func writeClipboardText(owner uintptr, value string) error {
	encoded, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	size := uintptr(len(encoded) * 2)
	handle, _, callErr := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return fmt.Errorf("allocate clipboard memory: %w", callErr)
	}
	transferred := false
	defer func() {
		if !transferred {
			procGlobalFree.Call(handle)
		}
	}()
	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		return fmt.Errorf("lock clipboard memory: %w", callErr)
	}
	procRtlMoveMemory.Call(pointer, uintptr(unsafe.Pointer(&encoded[0])), size)
	runtime.KeepAlive(encoded)
	procGlobalUnlock.Call(handle)
	if result, _, callErr := procOpenClipboard.Call(owner); result == 0 {
		return fmt.Errorf("open clipboard: %w", callErr)
	}
	defer procCloseClipboard.Call()
	if result, _, callErr := procEmptyClipboard.Call(); result == 0 {
		return fmt.Errorf("empty clipboard: %w", callErr)
	}
	if result, _, callErr := procSetClipboardData.Call(cfUnicodeText, handle); result == 0 {
		if errors.Is(callErr, syscall.Errno(0)) {
			callErr = errors.New("SetClipboardData returned zero")
		}
		return fmt.Errorf("set clipboard data: %w", callErr)
	}
	transferred = true
	return nil
}

func clipboardFormat(name string) uintptr {
	v, _, _ := clipUser.NewProc("RegisterClipboardFormatW").Call(uintptr(unsafe.Pointer(utf16Ptr(name))))
	return v
}

func clipboardSequence() uintptr { v, _, _ := clipSequence.Call(); return v }
func clipboardDesktopAvailable() bool {
	h, _, _ := clipUser.NewProc("OpenInputDesktop").Call(0, 0, 1)
	if h == 0 {
		return false
	}
	defer clipUser.NewProc("CloseDesktop").Call(h)
	var name [128]uint16
	var needed uint32
	ok, _, _ := clipUser.NewProc("GetUserObjectInformationW").Call(h, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed)))
	return ok != 0 && syscall.UTF16ToString(name[:]) == "Default"
}
func readClipboardText(hwnd uintptr) (string, uintptr, error) {
	if !clipboardDesktopAvailable() {
		return "", 0, errClipboardUnavailable
	}
	opened, _, _ := clipOpen.Call(hwnd)
	if opened == 0 {
		return "", 0, errClipboardUnavailable
	}
	defer clipClose.Call()
	sequence := clipboardSequence()
	for _, name := range []string{"ExcludeClipboardContentFromMonitorProcessing", "Krasis.RemoteClipboard.v1"} {
		format := clipboardFormat(name)
		if format == 0 {
			return "", sequence, errClipboardUnavailable
		}
		has, _, _ := clipUser.NewProc("IsClipboardFormatAvailable").Call(format)
		if has != 0 {
			return "", sequence, errClipboardUnavailable
		}
	}
	if clipboardHasFormat(cfHDROP) {
		return "", sequence, errClipboardUnavailable
	}
	handle, _, _ := clipGet.Call(13)
	if handle == 0 {
		return "", sequence, errClipboardUnavailable
	}
	size, _, _ := clipSize.Call(handle)
	if size > 2*(peerupdate.MaxClipboardBytes+1) {
		return "", sequence, errClipboardSize
	}
	if size < 2 || size%2 != 0 {
		return "", sequence, errClipboardUnavailable
	}
	ptr, _, _ := clipLock.Call(handle)
	if ptr == 0 {
		return "", sequence, errClipboardUnavailable
	}
	defer clipUnlock.Call(handle)
	data := make([]uint16, int(size/2))
	clipCopy.Call(uintptr(unsafe.Pointer(&data[0])), ptr, size)
	runtime.KeepAlive(data)
	end := -1
	for i, v := range data {
		if v == 0 {
			end = i
			break
		}
	}
	if end < 0 {
		return "", sequence, errClipboardUnavailable
	}
	text := string(unicode16.Decode(data[:end]))
	if len(text) > peerupdate.MaxClipboardBytes {
		return "", sequence, errClipboardSize
	}
	return text, sequence, nil
}
func utf16Ptr(s string) *uint16 { return utf16(s) }

func writeRemoteClipboardText(hwnd, expected uintptr, text, id string) (uintptr, error) {
	if !clipboardDesktopAvailable() {
		return 0, errClipboardUnavailable
	}
	data, err := syscall.UTF16FromString(text)
	if err != nil || len(text) > peerupdate.MaxClipboardBytes {
		return 0, errClipboardUnavailable
	}
	handle, _, _ := clipAlloc.Call(2, uintptr(len(data)*2))
	if handle == 0 {
		return 0, errClipboardUnavailable
	}
	owned := true
	defer func() {
		if owned {
			clipFree.Call(handle)
		}
	}()
	marker, _, _ := clipAlloc.Call(2, uintptr(len(id)+1))
	if marker == 0 {
		return 0, errClipboardUnavailable
	}
	markerOwned := true
	defer func() {
		if markerOwned {
			clipFree.Call(marker)
		}
	}()
	markerPtr, _, _ := clipLock.Call(marker)
	if markerPtr == 0 {
		return 0, errClipboardUnavailable
	}
	markerData := append([]byte(id), 0)
	clipCopy.Call(markerPtr, uintptr(unsafe.Pointer(&markerData[0])), uintptr(len(markerData)))
	runtime.KeepAlive(markerData)
	clipUnlock.Call(marker)
	markerFormat := clipboardFormat("Krasis.RemoteClipboard.v1")
	if markerFormat == 0 {
		return 0, errClipboardUnavailable
	}
	ptr, _, _ := clipLock.Call(handle)
	if ptr == 0 {
		return 0, errClipboardUnavailable
	}
	clipCopy.Call(ptr, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	runtime.KeepAlive(data)
	clipUnlock.Call(handle)
	ok, _, _ := clipOpen.Call(hwnd)
	if ok == 0 {
		return 0, errClipboardUnavailable
	}
	defer clipClose.Call()
	if clipboardSequence() != expected {
		return 0, errClipboardUnavailable
	}
	if ok, _, _ := clipEmpty.Call(); ok == 0 {
		return 0, errClipboardUnavailable
	}
	if ok, _, _ := clipSet.Call(13, handle); ok == 0 {
		return 0, errClipboardUnavailable
	}
	owned = false
	if ok, _, _ := clipSet.Call(markerFormat, marker); ok != 0 {
		markerOwned = false
	}
	return clipboardSequence(), nil
}

// Only the serialized GUI operation worker touches this state. Remote writes
// record their exact native sequence, so they never become a new outbound copy.
type clipboardPump struct {
	localChanged bool
	pendingAck   string
	peer         string
	sequence     uintptr
}

func (a *productWindow) syncClipboard(ctx context.Context) {
	a.clipPump.sync(ctx, a.hwnd, a.control, a.view.Update.Status.ClipboardPeer, a.view.Fingerprints)
}

func (p *clipboardPump) sync(ctx context.Context, hwnd uintptr, control, peer string, fingerprints map[string]string) {
	if peer == "" {
		*p = clipboardPump{}
		return
	}
	paired := false
	for _, fp := range fingerprints {
		if fp == peer {
			paired = true
		}
	}
	if !paired {
		*p = clipboardPump{}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !clipboardDesktopAvailable() {
		*p = clipboardPump{}
		return
	}
	current := clipboardSequence()
	if peer != p.peer {
		*p = clipboardPump{peer: peer, sequence: current}
	}
	previousSequence := p.sequence
	client, err := localapi.OpenClient(control)
	if err != nil {
		return
	}
	report := func(id, format, code string) {
		_, _ = client.Clipboard(ctx, localapi.ClipboardRequest{Action: "REPORT", Peer: peer, Format: format, Ack: id, Report: code})
	}
	request := localapi.ClipboardRequest{Action: "SYNC", Peer: peer}
	if current != p.sequence {
		if !clipboardHasFormat(clipboardFormat("Krasis.RemoteClipboard.v1")) {
			p.localChanged = true
		}
		localCandidate := !clipboardHasFormat(clipboardFormat("Krasis.RemoteClipboard.v1")) && !clipboardHasFormat(clipboardFormat("ExcludeClipboardContentFromMonitorProcessing"))
		fileCandidate := localCandidate && clipboardHasFormat(cfHDROP)
		if fileCandidate {
			report("", "files", "preparing")
		}
		format, data, sequence, present, err := readClipboardFiles(ctx, hwnd)
		if fileCandidate && err != nil {
			report("", "files", clipboardFileFailure(err))
		}
		if !present {
			format, data, sequence, present, err = readClipboardImage(hwnd)
			if localCandidate && present && err != nil && (format == "png" || format == "dib") {
				report("", format, clipboardPreparationFailure(err))
			}
		}
		if sequence != 0 {
			p.sequence = sequence
		}
		if present {
			if err == nil {
				request.Format, request.Data = format, data
				current = sequence
			}
		} else {
			text, textSequence, textErr := readClipboardText(hwnd)
			if textSequence != 0 {
				p.sequence = textSequence
			}
			if textErr == nil {
				request.Text = &text
				current = textSequence
			} else if localCandidate && clipboardHasFormat(cfUnicodeText) {
				report("", "", clipboardPreparationFailure(textErr))
			}
		}
	}
	request.LocalChanged = p.localChanged
	send := func(request localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		return client.Clipboard(ctx, request)
	}
	result, err := p.exchangeClipboard(ctx, previousSequence, request, send)
	if err != nil || result.Code != "ok" || result.Value.ID == "" {
		return
	}
	var sequence uintptr
	if ctx.Err() != nil {
		return
	}
	if result.Value.Format == "" {
		sequence, err = writeRemoteClipboardText(hwnd, current, result.Value.Text, result.Value.ID)
	} else if result.Value.Format == "files" {
		sequence, err = writeRemoteClipboardFiles(hwnd, current, result.Value.Data, result.Value.ID)
	} else {
		sequence, err = writeRemoteClipboardImage(hwnd, current, result.Value.Format, result.Value.Data, result.Value.ID)
	}
	if err != nil {
		report(result.Value.ID, result.Value.Format, "apply_failed")
		return
	}
	p.sequence = sequence
	p.pendingAck = result.Value.ID
	_, _ = p.exchangeClipboard(ctx, p.sequence, localapi.ClipboardRequest{Action: "SYNC", Peer: peer, Ack: result.Value.ID}, send)
}

func clipboardPreparationFailure(err error) string {
	if errors.Is(err, errClipboardSize) {
		return "size_limit"
	}
	return "prepare_failed"
}

func (a *productWindow) toggleClipboard() {
	if a.busy || a.closing || a.pin.job != "" {
		return
	}
	peer := a.view.Fingerprints[a.selected()]
	if active := a.view.Update.Status.ClipboardPeer; active != "" {
		a.performTarget("clipboard_disable", active)
		return
	}
	if len(peer) != 64 {
		return
	}
	action := "clipboard_enable"
	if a.view.Update.Status.ClipboardPeer == peer {
		action = "clipboard_disable"
	} else {
		text := "선택한 기기와 복사한 텍스트·이미지를 자동 공유할까요?\n\n비밀번호 등 민감한 내용도 포함될 수 있습니다. 양쪽에서 켜야 동작하며, 버튼을 다시 누르면 끕니다. 기존 클립보드 내용은 보내지 않습니다."
		result, _, _ := clipUser.NewProc("MessageBoxW").Call(a.hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr("클립보드 공유"))), 0x134)
		if result != 6 {
			return
		}
	}
	a.performTarget(action, peer)
}
