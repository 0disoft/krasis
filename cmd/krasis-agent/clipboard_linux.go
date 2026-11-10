package main

import (
	"context"
	"fmt"
	"github.com/0disoft/krasis/internal/linuxclipboard"
	"github.com/0disoft/krasis/internal/productidentity"
	"os"
	"time"
)

// Lives with the resident, not the short-lived CLI. Sharing remains opt-in.
func startDesktopServices(parent context.Context, base string) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		paths, err := productidentity.ResolveWindowsPaths(base)
		if err != nil {
			return
		}
		for ctx.Err() == nil {
			err = linuxclipboard.Run(ctx, linuxclipboard.Options{ControlDirectory: paths.Control})
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "clipboard_unavailable: X11 clipboard bridge stopped")
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}
