//go:build !linux

package linuxclipboard

import (
	"context"
	"errors"
)

func openClipboard(context.Context, string) (clipboard, error) {
	return nil, errors.New("clipboard bridge requires Linux X11 (DISPLAY and XFIXES)")
}
