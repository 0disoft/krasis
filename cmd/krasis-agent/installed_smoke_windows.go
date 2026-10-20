//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputworker"
)

// Fixed local maintenance smoke. It cannot run beside the resident, select a
// peer, desktop or path, or inject a key/button. The service authenticates this
// actual protected installed agent before any same-point cursor transaction.
func runInstalledInputSmoke() int {
	if err := installedInputSmoke(); err != nil {
		fmt.Fprintln(os.Stderr, "installed_input_smoke_failed:", err)
		return 1
	}
	fmt.Println("PASS: installed service/native child, cursor read/same-point placement, recovery shutdown")
	return 0
}

func installedInputSmoke() error {
	instance, err := agentinstance.Acquire()
	if err != nil {
		return err
	}
	defer instance.Close()
	service, err := inputworker.OpenAgentService()
	if err != nil {
		return err
	}
	defer service.Close()
	binding, err := inputworker.NewBinding()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := service.ConnectLease(ctx, inputworker.Lease{Identity: application.LeaseIdentity{SessionID: "local-native-smoke", LeaseID: "local-native-smoke", Epoch: "local-native-smoke"}, Binding: binding, ExpiresAt: time.Now().Add(15 * time.Second)})
	if err != nil {
		return err
	}
	defer client.Close()
	x, y, err := client.CursorPosition(ctx)
	if err != nil {
		return err
	}
	position, err := client.PlaceCursor(ctx, x, y)
	if err != nil {
		return err
	}
	if position.X != x || position.Y != y {
		return inputworker.ErrRecovery
	}
	return client.Close()
}
