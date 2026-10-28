//go:build !windows

package peerupdate

import "os"

func pinCurrentUIPath(string, bool) (*os.File, error) { return nil, ErrDenied }
