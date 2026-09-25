//go:build windows

package launcherui

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/png"
	"runtime"
	"unsafe"

	"github.com/0disoft/krasis/internal/peerupdate"
)

const (
	cfDIB   = 8
	cfHDROP = 15
	cfDIBV5 = 17
)

func clipboardHasFormat(format uintptr) bool {
	v, _, _ := clipUser.NewProc("IsClipboardFormatAvailable").Call(format)
	return v != 0
}

// readClipboardImage also reports whether an image or file format was present.
// That prevents a file path or broken image fallback from being sent as text.
func readClipboardImage(hwnd uintptr) (string, []byte, uintptr, bool, error) {
	if !clipboardDesktopAvailable() {
		return "", nil, 0, false, errClipboardUnavailable
	}
	if ok, _, _ := clipOpen.Call(hwnd); ok == 0 {
		return "", nil, 0, false, errClipboardUnavailable
	}
	defer clipClose.Call()
	sequence := clipboardSequence()
	for _, name := range []string{"ExcludeClipboardContentFromMonitorProcessing", "Krasis.RemoteClipboard.v1"} {
		format := clipboardFormat(name)
		if format == 0 || clipboardHasFormat(format) {
			return "", nil, sequence, true, errClipboardUnavailable
		}
	}
	if clipboardHasFormat(cfHDROP) {
		return "", nil, sequence, true, errClipboardUnavailable
	}
	pngFormat := clipboardFormat("PNG")
	present := false
	for _, item := range []struct {
		format uintptr
		kind   string
	}{{pngFormat, "png"}, {cfDIBV5, "dib"}, {cfDIB, "dib"}} {
		if item.format == 0 || !clipboardHasFormat(item.format) {
			continue
		}
		present = true
		data, err := readClipboardGlobalBytes(item.format)
		if err == nil && peerupdate.ValidClipboardImage(item.kind, data) {
			return item.kind, data, sequence, true, nil
		}
	}
	return "", nil, sequence, present, errClipboardUnavailable
}

func readClipboardGlobalBytes(format uintptr) ([]byte, error) {
	handle, _, _ := clipGet.Call(format)
	if handle == 0 {
		return nil, errClipboardUnavailable
	}
	size, _, _ := clipSize.Call(handle)
	if size == 0 || size > peerupdate.MaxClipboardImageBytes {
		return nil, errClipboardUnavailable
	}
	pointer, _, _ := clipLock.Call(handle)
	if pointer == 0 {
		return nil, errClipboardUnavailable
	}
	defer clipUnlock.Call(handle)
	data := make([]byte, int(size))
	clipCopy.Call(uintptr(unsafe.Pointer(&data[0])), pointer, size)
	return data, nil
}

func dibFromPNG(encoded []byte) ([]byte, error) {
	decoded, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		return nil, errClipboardUnavailable
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || int64(width)*int64(height)*4+40 > peerupdate.MaxClipboardImageBytes {
		return nil, errClipboardUnavailable
	}
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
	dib := make([]byte, 40+width*height*4)
	binary.LittleEndian.PutUint32(dib[:4], 40)
	binary.LittleEndian.PutUint32(dib[4:8], uint32(width))
	binary.LittleEndian.PutUint32(dib[8:12], uint32(height))
	binary.LittleEndian.PutUint16(dib[12:14], 1)
	binary.LittleEndian.PutUint16(dib[14:16], 32)
	binary.LittleEndian.PutUint32(dib[20:24], uint32(width*height*4))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			source := rgba.PixOffset(x, y)
			target := 40 + ((height-1-y)*width+x)*4
			dib[target+0] = rgba.Pix[source+2]
			dib[target+1] = rgba.Pix[source+1]
			dib[target+2] = rgba.Pix[source+0]
			dib[target+3] = rgba.Pix[source+3]
		}
	}
	return dib, nil
}

func clipboardGlobalBytes(data []byte) (uintptr, error) {
	handle, _, _ := clipAlloc.Call(gmemMoveable, uintptr(len(data)))
	if handle == 0 {
		return 0, errClipboardUnavailable
	}
	pointer, _, _ := clipLock.Call(handle)
	if pointer == 0 {
		clipFree.Call(handle)
		return 0, errClipboardUnavailable
	}
	clipCopy.Call(pointer, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	runtime.KeepAlive(data)
	clipUnlock.Call(handle)
	return handle, nil
}

func writeRemoteClipboardImage(hwnd, expected uintptr, format string, data []byte, id string) (uintptr, error) {
	if !clipboardDesktopAvailable() || !peerupdate.ValidClipboardImage(format, data) {
		return 0, errClipboardUnavailable
	}
	dib := data
	if format == "png" {
		var err error
		dib, err = dibFromPNG(data)
		if err != nil {
			return 0, err
		}
	}
	dibFormat := uintptr(cfDIB)
	if binary.LittleEndian.Uint32(dib[:4]) == 124 {
		dibFormat = cfDIBV5
	}
	type offered struct {
		format uintptr
		handle uintptr
	}
	var handles []offered
	defer func() {
		for _, item := range handles {
			if item.handle != 0 {
				clipFree.Call(item.handle)
			}
		}
	}()
	markerFormat := clipboardFormat("Krasis.RemoteClipboard.v1")
	if markerFormat == 0 {
		return 0, errClipboardUnavailable
	}
	marker, err := clipboardGlobalBytes(append([]byte(id), 0))
	if err != nil {
		return 0, err
	}
	handles = append(handles, offered{markerFormat, marker})
	imageHandle, err := clipboardGlobalBytes(dib)
	if err != nil {
		return 0, err
	}
	handles = append(handles, offered{dibFormat, imageHandle})
	if format == "png" {
		pngFormat := clipboardFormat("PNG")
		if pngFormat == 0 {
			return 0, errClipboardUnavailable
		}
		pngHandle, err := clipboardGlobalBytes(data)
		if err != nil {
			return 0, err
		}
		handles = append(handles, offered{pngFormat, pngHandle})
	}
	if ok, _, _ := clipOpen.Call(hwnd); ok == 0 {
		return 0, errClipboardUnavailable
	}
	defer clipClose.Call()
	if clipboardSequence() != expected {
		return 0, errClipboardUnavailable
	}
	if ok, _, _ := clipEmpty.Call(); ok == 0 {
		return 0, errClipboardUnavailable
	}
	for i := range handles {
		if ok, _, _ := clipSet.Call(handles[i].format, handles[i].handle); ok == 0 {
			return 0, errClipboardUnavailable
		}
		handles[i].handle = 0 // Windows now owns the HGLOBAL.
	}
	return clipboardSequence(), nil
}
