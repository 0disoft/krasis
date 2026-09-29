package residentagent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/truststore"
	"github.com/0disoft/krasis/internal/winlifecycle"
)

type instanceLease interface{ Close() error }

type apiServer interface {
	Run(context.Context) error
	Close(context.Context) error
}

type runDependencies struct {
	acquire         func() (instanceLease, error)
	bootstrap       func(string, time.Time) (productidentity.Paths, *productidentity.Identity, bool, error)
	startMonitor    agentlifecycle.StartMonitor
	listen          func(string, localapi.Operations) (apiServer, error)
	runner          SessionRunner
	pairing         PairWorkflow
	now             func() time.Time
	productAdapters bool
	log             *productlog.Logger
}

func Run(ctx context.Context, localAppData string) (err error) {
	log, err := productlog.Open(localAppData, "agent")
	if err != nil {
		return fmt.Errorf("initialize diagnostics: %w", err)
	}
	log.Event("process_started")
	defer func() {
		if err != nil {
			log.Event("process_failed")
		} else {
			log.Event("process_stopped")
		}
		log.Close()
	}()
	dependencies := runDependencies{
		log:          log,
		acquire:      func() (instanceLease, error) { return agentinstance.Acquire() },
		bootstrap:    productidentity.BootstrapWindows,
		startMonitor: winlifecycle.StartMonitor,
		listen: func(directory string, operations localapi.Operations) (apiServer, error) {
			return localapi.Listen(directory, operations)
		},
		now:             time.Now,
		productAdapters: true,
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
	defer func() {
		// Drain logs before releasing agent ownership so an upgrade cannot race
		// the previous process's log writer.
		if err != nil {
			dependencies.log.Event("process_failed")
		} else {
			dependencies.log.Event("process_stopped")
		}
		dependencies.log.Close()
		err = errors.Join(err, instance.Close())
	}()

	paths, identity, _, err := dependencies.bootstrap(localAppData, dependencies.now().UTC())
	if err != nil {
		return err
	}
	lifecycle, err := agentlifecycle.Start(ctx, dependencies.startMonitor)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lifecycle.Close()) }()
	lifecycleObserved := observeLifecycleStop(lifecycle.Context(), dependencies.log.Event)
	defer func() { lifecycle.Stop(); <-lifecycleObserved }()

	trust, err := truststore.New(paths.Trust)
	if err != nil {
		return err
	}
	if dependencies.productAdapters {
		identities, err := productidentity.New(paths.Identity)
		if err != nil {
			return err
		}
		dependencies.pairing = &networkPairing{root: lifecycle.Context(), identity: identity, identities: identities, trust: trust}
		dependencies.runner = productRunner(identities, trust, dependencies.log)
	}
	operations, err := New(lifecycle.Context(), identity.Public(), dependencies.runner, dependencies.pairing, trust)
	if err != nil {
		return err
	}
	operations.log = dependencies.log
	operations.localAppData = localAppData
	if dependencies.productAdapters {
		identities, err := productidentity.New(paths.Identity)
		if err != nil {
			return err
		}
		operations.preflight = func(alias string) error { _, err := prepareProductConnection(identities, trust, alias); return err }
		operations.credentials = identities
		updates, updateErr := peerupdate.Open(paths.Root, localapi.AgentGeneration)
		if updateErr == nil {
			updates.Observe = dependencies.log.Event
			operations.updates = updates
			updates.ReceiveDiagnostic = func(peer string, data []byte) (string, error) {
				return productlog.SaveRemoteSupport(localAppData, peer, data)
			}
			updates.ExportDiagnostic = operations.exportDiagnosticForRequest
			defer updates.Close()
			managementCtx, stopManagement := context.WithCancel(operations.Context())
			managementDone := make(chan struct{})
			go func() { defer close(managementDone); operations.maintainUpdates(managementCtx, identities, trust) }()
			defer func() { stopManagement(); <-managementDone }()
		}
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
	dependencies.log.Event("agent_ready")
	if err := server.Run(operations.Context()); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run resident agent local API: %w", err)
	}
	return nil
}
