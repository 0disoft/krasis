//go:build linux

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// logind terminates a whole session scope, including the recovery child.
// Retain the journal until the parent closes its pipe; only then may recovery
// release the held inputs. Releasing directly on SIGTERM could race a parent
// that is still injecting. ServeWithResult already bounds the EOF recovery.
func preserveRecoveryUntilParentEOF() func() {
	termination := make(chan os.Signal, 1)
	signal.Notify(termination, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	return func() { signal.Stop(termination) }
}
