//go:build windows

package inputworker

import (
	"crypto/sha256"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Compute identical full SHA-256 values in 32 KiB chunks. In-memory fixture,
// not file/installation/whole-worker evidence. Includes native handle cleanup.
func BenchmarkImageSHAProvider(b *testing.B) {
	data := make([]byte, 8<<20)
	data[0], data[len(data)-1] = 1, 2
	expected := sha256.Sum256(data)
	dll := windows.NewLazySystemDLL("bcrypt.dll")
	open := dll.NewProc("BCryptOpenAlgorithmProvider")
	close := dll.NewProc("BCryptCloseAlgorithmProvider")
	create := dll.NewProc("BCryptCreateHash")
	destroy := dll.NewProc("BCryptDestroyHash")
	write := dll.NewProc("BCryptHashData")
	finish := dll.NewProc("BCryptFinishHash")
	algorithm, _ := windows.UTF16PtrFromString("SHA256")
	provider, _ := windows.UTF16PtrFromString("Microsoft Primitive Provider")
	b.Run("Go", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		for b.Loop() {
			hash := sha256.New()
			for offset := 0; offset < len(data); offset += 32 << 10 {
				_, _ = hash.Write(data[offset : offset+(32<<10)])
			}
			var digest [sha256.Size]byte
			copy(digest[:], hash.Sum(nil))
			if digest != expected {
				b.Fatal("incorrect Go digest")
			}
		}
	})
	b.Run("WindowsCNG", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		for b.Loop() {
			var handle, state windows.Handle
			status, _, _ := open.Call(uintptr(unsafe.Pointer(&handle)), uintptr(unsafe.Pointer(algorithm)), uintptr(unsafe.Pointer(provider)), 0)
			if status != 0 {
				b.Fatal("native algorithm unavailable")
			}
			status, _, _ = create.Call(uintptr(handle), uintptr(unsafe.Pointer(&state)), 0, 0, 0, 0, 0)
			if status != 0 {
				close.Call(uintptr(handle), 0)
				b.Fatal("native state unavailable")
			}
			for offset := 0; offset < len(data); offset += 32 << 10 {
				status, _, _ = write.Call(uintptr(state), uintptr(unsafe.Pointer(&data[offset])), 32<<10, 0)
				runtime.KeepAlive(data)
				if status != 0 {
					destroy.Call(uintptr(state))
					close.Call(uintptr(handle), 0)
					b.Fatal("native hash failed")
				}
			}
			var digest [sha256.Size]byte
			status, _, _ = finish.Call(uintptr(state), uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)), 0)
			destroyStatus, _, _ := destroy.Call(uintptr(state))
			closeStatus, _, _ := close.Call(uintptr(handle), 0)
			runtime.KeepAlive(digest)
			if status != 0 || destroyStatus != 0 || closeStatus != 0 || digest != expected {
				b.Fatal("native digest or cleanup failed")
			}
		}
	})
}
