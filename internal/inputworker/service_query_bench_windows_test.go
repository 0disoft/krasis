//go:build windows

package inputworker

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Read-only baseline: a native test pipe plus the installed EventLog service.
// The service does not own that pipe; this measures queries, not authority.
// Both handles live until every subbenchmark has finished; no input, service
// control, token, configuration or access-policy writes are performed.
func BenchmarkServiceQueries(b *testing.B) {
	_, pipe := nativePair(b)
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		b.Fatal(err)
	}
	defer windows.CloseServiceHandle(manager)
	name, _ := windows.UTF16PtrFromString("EventLog")
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		b.Fatal(err)
	}
	defer windows.CloseServiceHandle(service)
	query := func() error {
		var status windows.SERVICE_STATUS_PROCESS
		var needed uint32
		return windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed)
	}
	b.Run("pipe-owner", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var pid uint32
			if err := windows.GetNamedPipeServerProcessId(pipe.handle, &pid); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("service-status", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := query(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("sequential", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := readServicePipeProcess(pipe.handle, service); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("overlapped", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			// Exactly one extra query, always joined, even if the owner read
			// fails. Do not let a benchmark leak work into a later iteration.
			done := make(chan error, 1)
			go func() { done <- query() }()
			var pid uint32
			ownerErr := windows.GetNamedPipeServerProcessId(pipe.handle, &pid)
			statusErr := <-done
			if ownerErr != nil {
				b.Fatal(ownerErr)
			}
			if statusErr != nil {
				b.Fatal(statusErr)
			}
		}
	})
}
