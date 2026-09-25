//go:build windows

package launcherui

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/peerupdate"
)

var dragQueryFile = syscall.NewLazyDLL("shell32.dll").NewProc("DragQueryFileW")

func readClipboardFiles(hwnd uintptr) (string, []byte, uintptr, bool, error) {
	paths, sequence, present, err := clipboardSelectedPaths(hwnd)
	if err != nil || !present {
		return "", nil, sequence, present, err
	}
	data, err := packClipboardFiles(paths)
	if err != nil || clipboardSequence() != sequence {
		return "", nil, sequence, true, errClipboardUnavailable
	}
	return "files", data, sequence, true, nil
}

func clipboardSelectedPaths(hwnd uintptr) ([]string, uintptr, bool, error) {
	if !clipboardDesktopAvailable() {
		return nil, 0, false, errClipboardUnavailable
	}
	if ok, _, _ := clipOpen.Call(hwnd); ok == 0 {
		return nil, 0, false, errClipboardUnavailable
	}
	defer clipClose.Call()
	sequence := clipboardSequence()
	for _, name := range []string{"ExcludeClipboardContentFromMonitorProcessing", "Krasis.RemoteClipboard.v1"} {
		format := clipboardFormat(name)
		if format == 0 || clipboardHasFormat(format) {
			return nil, sequence, true, errClipboardUnavailable
		}
	}
	if !clipboardHasFormat(cfHDROP) {
		return nil, sequence, false, nil
	}
	handle, _, _ := clipGet.Call(cfHDROP)
	if handle == 0 {
		return nil, sequence, true, errClipboardUnavailable
	}
	count, _, _ := dragQueryFile.Call(handle, ^uintptr(0), 0, 0)
	if count == 0 || count > 64 {
		return nil, sequence, true, errClipboardUnavailable
	}
	paths := make([]string, 0, count)
	for i := uintptr(0); i < count; i++ {
		n, _, _ := dragQueryFile.Call(handle, i, 0, 0)
		if n == 0 || n > 32767 {
			return nil, sequence, true, errClipboardUnavailable
		}
		buf := make([]uint16, n+1)
		got, _, _ := dragQueryFile.Call(handle, i, uintptr(unsafe.Pointer(&buf[0])), n+1)
		if got != n {
			return nil, sequence, true, errClipboardUnavailable
		}
		paths = append(paths, syscall.UTF16ToString(buf))
	}
	return paths, sequence, true, nil
}

func packClipboardFiles(paths []string) ([]byte, error) {
	if len(paths) == 0 || len(paths) > 64 {
		return nil, errClipboardUnavailable
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entries := 0
	var total int64
	for _, source := range paths {
		if !filepath.IsAbs(source) {
			return nil, errClipboardUnavailable
		}
		parent := filepath.Dir(source)
		err := filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entries >= peerupdate.MaxClipboardFiles {
				return errClipboardUnavailable
			}
			info, err := os.Lstat(current)
			if err != nil || !info.IsDir() && !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return errClipboardUnavailable
			}
			wide, err := syscall.UTF16PtrFromString(current)
			if err != nil {
				return errClipboardUnavailable
			}
			attrs, err := syscall.GetFileAttributes(wide)
			if err != nil || attrs&0x400 != 0 { // FILE_ATTRIBUTE_REPARSE_POINT
				return errClipboardUnavailable
			}
			rel, err := filepath.Rel(parent, current)
			if err != nil {
				return errClipboardUnavailable
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return errClipboardUnavailable
			}
			header.Name = filepath.ToSlash(rel)
			if info.IsDir() {
				header.Name += "/"
				header.Method = zip.Store
			} else {
				if info.Size() < 0 || total+info.Size() > peerupdate.MaxClipboardFileBytes {
					return errClipboardUnavailable
				}
				header.Method = zip.Deflate
			}
			writer, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}
			entries++
			if !info.IsDir() {
				file, err := os.Open(current)
				if err != nil {
					return err
				}
				n, copyErr := io.Copy(writer, io.LimitReader(file, peerupdate.MaxClipboardFileBytes-total+1))
				closeErr := file.Close()
				if copyErr != nil || closeErr != nil || n != info.Size() || n > peerupdate.MaxClipboardFileBytes-total {
					return errClipboardUnavailable
				}
				total += n
			}
			return nil
		})
		if err != nil {
			return nil, errClipboardUnavailable
		}
	}
	if archive.Close() != nil || !peerupdate.ValidClipboardFiles(buffer.Bytes()) {
		return nil, errClipboardUnavailable
	}
	return buffer.Bytes(), nil
}

func clipboardFilesRoot() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" || !filepath.IsAbs(base) {
		return "", errClipboardUnavailable
	}
	root := filepath.Join(base, "Krasis", "clipboard-files")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if _, err := peercredentials.ProtectSecurityDirectory(root); err != nil {
		return "", err
	}
	return root, nil
}

func extractClipboardFiles(data []byte) (string, []string, error) {
	base, err := clipboardFilesRoot()
	if err != nil {
		return "", nil, err
	}
	return extractClipboardFilesAt(base, data)
}

func extractClipboardFilesAt(base string, data []byte) (string, []string, error) {
	if !peerupdate.ValidClipboardFiles(data) {
		return "", nil, errClipboardUnavailable
	}
	cleanupClipboardFiles(base)
	stage, err := os.MkdirTemp(base, "copy-")
	if err != nil {
		return "", nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	if _, err := peercredentials.ProtectSecurityDirectory(stage); err != nil {
		return "", nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", nil, err
	}
	var roots []string
	var total int64
	for _, item := range reader.File {
		name := strings.TrimSuffix(item.Name, "/")
		target := filepath.Join(stage, filepath.FromSlash(name))
		if !strings.Contains(name, "/") {
			roots = append(roots, target)
		}
		if item.FileInfo().IsDir() {
			if err := os.Mkdir(target, 0700); err != nil {
				return "", nil, err
			}
			continue
		}
		input, err := item.Open()
		if err != nil {
			return "", nil, err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return "", nil, err
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, peerupdate.MaxClipboardFileBytes-total+1))
		closeErr := output.Close()
		readErr := input.Close()
		if copyErr != nil || closeErr != nil || readErr != nil || n != int64(item.UncompressedSize64) || n > peerupdate.MaxClipboardFileBytes-total {
			return "", nil, errClipboardUnavailable
		}
		total += n
	}
	keep = true
	return stage, roots, nil
}

func cleanupClipboardFiles(base string) {
	items, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, item := range items {
		if !item.IsDir() || !strings.HasPrefix(item.Name(), "copy-") {
			continue
		}
		info, err := item.Info()
		if err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(filepath.Join(base, item.Name()))
		}
	}
}

func hdropBytes(paths []string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, errClipboardUnavailable
	}
	data := make([]byte, 20)
	binary.LittleEndian.PutUint32(data[:4], 20)
	binary.LittleEndian.PutUint32(data[16:20], 1) // Unicode DROPFILES.
	for _, name := range paths {
		wide, err := syscall.UTF16FromString(name)
		if err != nil {
			return nil, err
		}
		for _, unit := range wide {
			data = binary.LittleEndian.AppendUint16(data, unit)
		}
	}
	return append(data, 0, 0), nil
}

func writeRemoteClipboardFiles(hwnd, expected uintptr, data []byte, id string) (uintptr, error) {
	if !clipboardDesktopAvailable() {
		return 0, errClipboardUnavailable
	}
	stage, paths, err := extractClipboardFiles(data)
	if err != nil {
		return 0, err
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(stage)
		}
	}()
	drop, err := hdropBytes(paths)
	if err != nil {
		return 0, err
	}
	markerFormat := clipboardFormat("Krasis.RemoteClipboard.v1")
	if markerFormat == 0 {
		return 0, errClipboardUnavailable
	}
	marker, err := clipboardGlobalBytes(append([]byte(id), 0))
	if err != nil {
		return 0, err
	}
	defer func() {
		if marker != 0 {
			clipFree.Call(marker)
		}
	}()
	fileHandle, err := clipboardGlobalBytes(drop)
	if err != nil {
		return 0, err
	}
	defer func() {
		if fileHandle != 0 {
			clipFree.Call(fileHandle)
		}
	}()
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
	if ok, _, _ := clipSet.Call(markerFormat, marker); ok == 0 {
		return 0, errClipboardUnavailable
	}
	marker = 0
	if ok, _, _ := clipSet.Call(cfHDROP, fileHandle); ok == 0 {
		return 0, errClipboardUnavailable
	}
	fileHandle = 0
	if effectFormat := clipboardFormat("Preferred DropEffect"); effectFormat != 0 {
		effect, allocErr := clipboardGlobalBytes([]byte{1, 0, 0, 0})
		if allocErr == nil {
			if ok, _, _ := clipSet.Call(effectFormat, effect); ok == 0 {
				clipFree.Call(effect)
			}
		}
	}
	installed = true
	runtime.KeepAlive(paths)
	return clipboardSequence(), nil
}
