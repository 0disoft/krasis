//go:build !windows

package securefs

func (*Directory) ReadNames() ([]string, error) { return nil, ErrUnsupportedPlatform }
