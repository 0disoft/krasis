//go:build windows

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/0disoft/krasis/internal/inputworker"
)

func main() {
	if len(os.Args) != 2 || !validInputMode(os.Args[1]) {
		fmt.Fprintln(os.Stderr, "invalid_input_mode")
		os.Exit(2)
	}
	var installation *inputworker.Installation
	var err error
	if os.Args[1] == "--service" || os.Args[1] == "--login-worker" {
		installation, err = inputworker.LoadInstallation()
	} else {
		installation, err = inputworker.LoadWorkerInstallation()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "input_installation_unavailable")
		os.Exit(1)
	}
	defer installation.Close()
	if os.Args[1] == "--service" {
		err = inputworker.RunService(installation)
	} else if os.Args[1] == "--login-worker" {
		err = inputworker.RunInstalledLoginConnection(context.Background(), installation)
	} else if os.Args[1] == "--native-input-worker" {
		err = inputworker.RunInstalledNativeInput(context.Background(), installation)
	} else {
		err = inputworker.RunInstalledWorker(context.Background(), installation)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "input_worker_failed")
		_ = installation.Close()
		os.Exit(1)
	}
}

func validInputMode(mode string) bool {
	return mode == "--service" || mode == "--input-worker" || mode == "--login-worker" || mode == "--native-input-worker"
}
