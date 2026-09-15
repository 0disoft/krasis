//go:build windows

package launcherui

import (
	"reflect"
	"syscall"
	"testing"
)

func TestAppIconLoadsRequestedSizeAndReleasesOnce(t *testing.T) {
	load, destroy := callLoadAppIcon, callDestroyAppIcon
	t.Cleanup(func() { callLoadAppIcon, callDestroyAppIcon = load, destroy })
	callLoadAppIcon = func(args ...uintptr) (uintptr, uintptr, error) {
		if !reflect.DeepEqual(args, []uintptr{7, 1, 1, 20, 20, 0}) {
			t.Fatalf("load arguments: %v", args)
		}
		return 42, 0, syscall.Errno(0)
	}
	calls := 0
	callDestroyAppIcon = func(args ...uintptr) (uintptr, uintptr, error) {
		calls++
		if !reflect.DeepEqual(args, []uintptr{42}) {
			t.Fatalf("destroy arguments: %v", args)
		}
		return 1, 0, syscall.Errno(0)
	}
	icon := loadAppIcon(7, 20, 20)
	if icon.handle != 42 || !icon.owned {
		t.Fatalf("icon = %+v", icon)
	}
	icon.close()
	icon.close()
	if calls != 1 {
		t.Fatalf("destroy calls = %d", calls)
	}
}

func TestAppIconFallbackRemainsShared(t *testing.T) {
	load, destroy := callLoadAppIcon, callDestroyAppIcon
	t.Cleanup(func() { callLoadAppIcon, callDestroyAppIcon = load, destroy })
	for _, fallback := range []uintptr{0, 99} {
		calls := 0
		callLoadAppIcon = func(args ...uintptr) (uintptr, uintptr, error) {
			calls++
			if calls == 1 {
				return 0, 0, syscall.Errno(1813)
			}
			if !reflect.DeepEqual(args, []uintptr{0, 32512, 1, 16, 16, 0x8000}) {
				t.Fatalf("fallback: %v", args)
			}
			return fallback, 0, syscall.Errno(0)
		}
		callDestroyAppIcon = func(...uintptr) (uintptr, uintptr, error) { t.Fatal("destroyed shared icon"); return 0, 0, nil }
		icon := loadAppIcon(7, 16, 16)
		if icon.handle != fallback || icon.owned || calls != 2 {
			t.Fatalf("fallback icon = %+v, calls=%d", icon, calls)
		}
		icon.close()
	}
}

func TestTrayUsesWindowSmallIcon(t *testing.T) {
	a := &productWindow{trayIcon: 42}
	if d := a.trayData(); d.Icon != 42 {
		t.Fatalf("tray icon = %d", d.Icon)
	}
}
