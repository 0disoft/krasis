//go:build windows

package evidence

import "unsafe"

func unsafePointer[T any](value *T) unsafe.Pointer {
	return unsafe.Pointer(value)
}
