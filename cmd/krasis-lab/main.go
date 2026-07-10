package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/0disoft/krasis/internal/labcli"
	"github.com/0disoft/krasis/internal/wininput"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	platform := wininput.NewPlatform()
	os.Exit(labcli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, platform))
}
