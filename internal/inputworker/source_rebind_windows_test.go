//go:build windows

package inputworker

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNativeSourceRebindWaitsForRetiredServiceEndpoint(t *testing.T) {
	// Retirement reaches the agent before service-side report/handle cleanup.
	// Reproduce that cleanup plus the old 200ms service backoff exceeding the
	// ordinary endpoint's 250ms grace. No lease or input may precede this open.
	name := fmt.Sprintf(`\\.\pipe\Krasis.Source.Test.%d.%s`, os.Getpid(), t.Name())
	ctx, cancel := context.WithTimeout(context.Background(), serviceConnectTimeout)
	defer cancel()
	type result struct {
		server *pipeConn
		err    error
	}
	done := make(chan result, 1)
	go func() {
		timer := time.NewTimer(350 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			done <- result{err: ctx.Err()}
		case <-timer.C:
			server, err := makePipe(name)
			if err == nil {
				err = server.connect(ctx)
			}
			done <- result{server, err}
		}
	}()
	client, err := openPipeWaiting(ctx, name, installedPipeMissingGrace(sourcePipeName))
	if client != nil {
		defer client.Close()
	}
	if err != nil {
		cancel()
	}
	accepted := <-done
	if accepted.server != nil {
		defer accepted.server.Close()
	}
	if err != nil || accepted.err != nil || client == nil {
		t.Fatal("joined source could not rebind after service cleanup", err, accepted.err)
	}
	var pid uint32
	if windows.GetNamedPipeServerProcessId(client.handle, &pid) != nil || pid != windows.GetCurrentProcessId() {
		t.Fatal("rebind bypassed the kernel server identity needed for authentication")
	}
}

func TestSourceRebindGraceKeepsOtherEndpointsAndEarlierDeadline(t *testing.T) {
	if installedPipeMissingGrace(pipeName) != servicePipeMissingGrace || installedPipeMissingGrace("worker-control") != servicePipeMissingGrace {
		t.Fatal("source rebind widened other installed endpoint waits")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := dialPipe(ctx, installedPipeMissingGrace(sourcePipeName), func() (*pipeConn, error) {
		return nil, windows.ERROR_FILE_NOT_FOUND
	}); err != context.DeadlineExceeded {
		t.Fatal("source rebind extended caller deadline", err)
	}
	calls := 0
	if _, err := dialPipe(context.Background(), installedPipeMissingGrace(sourcePipeName), func() (*pipeConn, error) {
		calls++
		return nil, windows.ERROR_ACCESS_DENIED
	}); err != windows.ERROR_ACCESS_DENIED || calls != 1 {
		t.Fatal("source rebind retried access denial", err, calls)
	}
}
