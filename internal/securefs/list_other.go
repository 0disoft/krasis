//go:build !windows

package securefs

import "os"

func (directory *Directory) ReadNames() ([]string, error) {
	f, err := os.Open(directory.path)
	if err != nil {
		return nil, err
	}
	names, readErr := f.Readdirnames(-1)
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	return names, closeErr
}
