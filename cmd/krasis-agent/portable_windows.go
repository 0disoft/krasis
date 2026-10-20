//go:build windows

package main

import (
	"context"
	"os"
	"strconv"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/wininput"
)

// These modes consume only inherited local stdio/process references. They do
// not create a network listener, peer authorization or privileged worker proof.
func runPortableChild(args []string) (bool, int) {
	if len(args) == 0 || args[0] != "product-portable-input-internal" && args[0] != "product-portable-guardian-internal" {
		return false, 0
	}
	guardian := args[0] == "product-portable-guardian-internal"
	var parent, input uint64
	if guardian {
		if len(args) != 3 {
			return true, 2
		}
		var err error
		parent, err = strconv.ParseUint(args[1], 10, strconv.IntSize)
		if err != nil || parent == 0 {
			return true, 2
		}
		input, err = strconv.ParseUint(args[2], 10, strconv.IntSize)
		if err != nil || input == 0 || input == parent {
			return true, 2
		}
	} else if len(args) != 1 {
		return true, 2
	}
	err := inputdesktop.Run(context.Background(), func(scope *inputdesktop.Scope) error {
		port := wininput.NewInjectionPort()
		if guardian {
			_, err := crashguard.ServePortableGuardian(uintptr(parent), uintptr(input), os.Stdin, os.Stdout, portableRelease{scope, port})
			return err
		}
		_, err := crashguard.ServeWithInput(os.Stdin, os.Stdout, portableRelease{scope, port}, inputworker.PortableInputHandler(port, scope.Do))
		return err
	})
	if err != nil {
		return true, 1
	}
	return true, 0
}

type portableRelease struct {
	scope *inputdesktop.Scope
	port  application.ReleasePort
}

func (release portableRelease) Release(ctx context.Context, token control.PressedToken) error {
	return release.scope.Do(ctx, func() error { return release.port.Release(ctx, token) })
}
