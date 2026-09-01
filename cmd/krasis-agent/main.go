package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/0disoft/krasis/internal/residentagent"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := residentagent.Run(ctx, os.Getenv("LOCALAPPDATA")); err != nil {
		fmt.Fprintln(os.Stderr, "agent_error:", err)
		os.Exit(1)
	}
}
