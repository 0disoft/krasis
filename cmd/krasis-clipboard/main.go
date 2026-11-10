// krasis-clipboard serves the interactive Linux X11 session clipboard while
// krasis-agent owns the authenticated peer connection. No external X tools or
// cgo libraries are required.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0disoft/krasis/internal/linuxclipboard"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stderr))
}

func run(ctx context.Context, args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("krasis-clipboard", flag.ContinueOnError)
	flags.SetOutput(stderr)
	control := flags.String("control-dir", "", "resident agent control directory (required)")
	display := flags.String("display", "", "X11 display (default DISPLAY; requires XFIXES)")
	interval := flags.Duration("poll-interval", 250*time.Millisecond, "clipboard sync interval (10ms..1s)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: krasis-clipboard --control-dir DIR [--display :0]")
		fmt.Fprintln(stderr, "Enable sharing first with: krasis-cli --control-dir DIR clipboard on ALIAS")
		fmt.Fprintln(stderr, "Supports UTF-8 text and PNG with bounded INCR in both directions. Files, DIB, native Wayland unsupported.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *control == "" || flags.NArg() != 0 || *interval < 10*time.Millisecond || *interval > time.Second {
		flags.Usage()
		return 2
	}
	var lastError time.Time
	err := linuxclipboard.Run(ctx, linuxclipboard.Options{ControlDirectory: *control, Display: *display, PollInterval: *interval, OnError: func(error) {
		if time.Since(lastError) >= 30*time.Second {
			fmt.Fprintln(stderr, "clipboard sync unavailable; retrying (check resident agent and sharing status)")
			lastError = time.Now()
		}
	}})
	if errors.Is(err, context.Canceled) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "clipboard bridge:", err)
		return 1
	}
	return 0
}
