//go:build windows

package wininput

import "testing"

func TestSourceCallbackTrampolinesSurviveOwnerChurn(t *testing.T) {
	keyboard, mouse, capture := sourceKeyboardCallback, sourceMouseCallback, sourceCaptureCallback
	for i := 0; i < 10000; i++ {
		owner := &sourceHookCallbacks{
			keyboard: func(int, uintptr, uintptr) uintptr { return 17 },
			mouse:    func(int, uintptr, uintptr) uintptr { return 19 },
		}
		key := ^uintptr(0)
		if _, loaded := sourceHookOwners.LoadOrStore(key, owner); loaded {
			t.Fatal("retained hook owner")
		}
		value, ok := sourceHookOwners.Load(key)
		if !ok || value.(*sourceHookCallbacks).keyboard(0, 0, 0) != 17 {
			t.Fatal("wrong dispatch owner")
		}
		if !sourceHookOwners.CompareAndDelete(key, owner) {
			t.Fatal("owner cleanup failed")
		}
		if _, ok := sourceHookOwners.Load(key); ok {
			t.Fatal("late callback can reach old owner")
		}
		window := sourceWindowCallback(func(uintptr, uint32, uintptr, uintptr) uintptr { return 23 })
		sourceWindowOwners.Store(key, window)
		value, ok = sourceWindowOwners.Load(key)
		if !ok || value.(sourceWindowCallback)(key, 0, 0, 0) != 23 {
			t.Fatal("wrong window owner")
		}
		sourceWindowOwners.Delete(key)
		if _, ok := sourceWindowOwners.Load(key); ok {
			t.Fatal("retained window owner")
		}
	}
	if sourceKeyboardCallback != keyboard || sourceMouseCallback != mouse || sourceCaptureCallback != capture {
		t.Fatal("callback trampoline changed across owner lifetimes")
	}
}
