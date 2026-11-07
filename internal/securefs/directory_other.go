//go:build !windows

package securefs

import "os"

// The product supports Windows only. A pathname-based fallback cannot uphold
// the handle-relative, no-reparse and atomic no-replace security guarantees.
type Directory struct{}

func OpenReadDirectory(string) (*Directory, error) { return nil, ErrUnsupportedPlatform }
func OpenDirectory(string) (*Directory, error)     { return nil, ErrUnsupportedPlatform }
func (*Directory) Close() error                    { return nil }
func (*Directory) Handle() uintptr                 { return 0 }
func (*Directory) Open(string) (*os.File, error)   { return nil, ErrUnsupportedPlatform }
func (*Directory) OpenObserved(string, func(string)) (*os.File, error) {
	return nil, ErrUnsupportedPlatform
}
func (*Directory) OpenReadWrite(string) (*os.File, error)  { return nil, ErrUnsupportedPlatform }
func (*Directory) OpenLock(string, bool) (*os.File, error) { return nil, ErrUnsupportedPlatform }
func (*Directory) CreateLock(string, os.FileMode) (*os.File, error) {
	return nil, ErrUnsupportedPlatform
}
func (*Directory) CreateExclusive(string, os.FileMode) (*os.File, error) {
	return nil, ErrUnsupportedPlatform
}
func (*Directory) CreateTemporary(string, os.FileMode) (*os.File, string, error) {
	return nil, "", ErrUnsupportedPlatform
}
func (*Directory) CreateDirectory(string, os.FileMode) (*Directory, error) {
	return nil, ErrUnsupportedPlatform
}
func (*Directory) Replace(string, string) error          { return ErrUnsupportedPlatform }
func (*Directory) PromoteNoReplace(string, string) error { return ErrUnsupportedPlatform }
func (*Directory) Remove(string) error                   { return ErrUnsupportedPlatform }
func (*Directory) RemoveSelf() error                     { return ErrUnsupportedPlatform }
