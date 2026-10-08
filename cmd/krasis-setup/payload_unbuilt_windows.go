//go:build windows && !appsetup

package main

import "github.com/0disoft/krasis/internal/appinstall"

// Generic builds cannot install anything; only the package builder embeds it.
func payload() ([]byte, error) { return nil, appinstall.ErrInvalid }
