package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/residentagent"
	"github.com/0disoft/krasis/internal/wininput"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "credential-ui-probe" {
		os.Exit(runCredentialProbe())
	}
	if err := wininput.InitializePhysicalCoordinates(); err != nil {
		fmt.Fprintln(os.Stderr, "agent_error: physical screen coordinates unavailable")
		os.Exit(1)
	}
	if len(os.Args) == 2 && os.Args[1] == "product-crash-guard-internal" {
		_, err := crashguard.ServeWithResult(os.Stdin, os.Stdout, wininput.NewInputAdapter())
		if err != nil {
			os.Exit(1)
		}
		return
	}
	var recovery []string
	if len(os.Args) == 3 && os.Args[1] == "--recovery" {
		recovery = []string{os.Args[2]}
	}
	if len(os.Args) != 1 && len(recovery) == 0 {
		fmt.Fprintln(os.Stderr, "invalid agent arguments")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := residentagent.Run(ctx, os.Getenv("LOCALAPPDATA"), recovery...); err != nil {
		fmt.Fprintln(os.Stderr, "agent_error: resident agent stopped")
		os.Exit(1)
	}
}
