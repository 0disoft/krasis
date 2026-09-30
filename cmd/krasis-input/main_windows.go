//go:build windows

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/0disoft/krasis/internal/inputworker"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "--service" && os.Args[1] != "--input-worker") {
		fmt.Fprintln(os.Stderr, "invalid_input_mode")
		os.Exit(2)
	}
	installation, err := inputworker.LoadInstallation()
	if err != nil {
		fmt.Fprintln(os.Stderr, "input_installation_unavailable")
		os.Exit(1)
	}
	defer installation.Close()
	if os.Args[1] == "--service" {
		err = inputworker.RunService(installation)
	} else {
		err = inputworker.RunInstalledWorker(context.Background(), installation)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "input_worker_failed")
		_ = installation.Close()
		os.Exit(1)
	}
}
