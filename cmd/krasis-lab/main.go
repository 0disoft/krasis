package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/labcli"
	"github.com/0disoft/krasis/internal/wininput"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	platform := wininput.NewPlatform()
	if len(os.Args) == 2 && os.Args[1] == "crash-guard-internal" {
		if err := crashguard.Serve(os.Stdin, os.Stdout, wininput.NewInputAdapter()); err != nil {
			os.Exit(1)
		}
		return
	}
	os.Exit(labcli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, platform))
}
