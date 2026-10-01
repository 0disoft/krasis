//go:build windows && !inputinstaller

package main

import "github.com/0disoft/krasis/internal/inputworker"

// Generic builds remain inert. Only the reviewed installer builder embeds
// clean command images and the requireAdministrator manifest.
func loadEmbeddedPayload() (installerPayload, error) {
	return installerPayload{}, inputworker.ErrAuthority
}
