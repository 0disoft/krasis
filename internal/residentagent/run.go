package residentagent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/winlifecycle"
)

type instanceLease interface{ Close() error }

type apiServer interface {
	Run(context.Context) error
	Close(context.Context) error
}

type runDependencies struct {
	acquire      func() (instanceLease, error)
	bootstrap    func(string, time.Time) (productidentity.Paths, *productidentity.Identity, bool, error)
	startMonitor agentlifecycle.StartMonitor
	listen       func(string, localapi.Operations) (apiServer, error)
	runner       SessionRunner
	pairing      PairWorkflow
	now          func() time.Time
}

func Run(ctx context.Context, localAppData string) error {
	dependencies := runDependencies{
		acquire:      func() (instanceLease, error) { return agentinstance.Acquire() },
		bootstrap:    productidentity.BootstrapWindows,
		startMonitor: winlifecycle.StartMonitor,
		listen: func(directory string, operations localapi.Operations) (apiServer, error) {
			return localapi.Listen(directory, operations)
		},
		now: time.Now,
	}
	return run(ctx, localAppData, dependencies)
}

func run(ctx context.Context, localAppData string, dependencies runDependencies) (err error) {
	if ctx == nil || localAppData == "" || dependencies.acquire == nil || dependencies.bootstrap == nil ||
		dependencies.startMonitor == nil || dependencies.listen == nil || dependencies.now == nil {
		return errors.New("resident agent dependencies and LOCALAPPDATA are required")
	}
	instance, err := dependencies.acquire()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, instance.Close()) }()

	paths, identity, _, err := dependencies.bootstrap(localAppData, dependencies.now().UTC())
	if err != nil {
		return err
	}
	lifecycle, err := agentlifecycle.Start(ctx, dependencies.startMonitor)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lifecycle.Close()) }()

	operations, err := New(lifecycle.Context(), identity.Public(), dependencies.runner, dependencies.pairing)
	if err != nil {
		return err
	}
	server, err := dependencies.listen(paths.Control, operations)
	if err != nil {
		return err
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), DefaultSessionStopTimeout)
		defer cancel()
		err = errors.Join(err, server.Close(shutdown), operations.Close(shutdown))
	}()
	if err := server.Run(operations.Context()); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run resident agent local API: %w", err)
	}
	return nil
}
