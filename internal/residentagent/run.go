package residentagent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/runtimestall"
	"github.com/0disoft/krasis/internal/truststore"
	"github.com/0disoft/krasis/internal/winlifecycle"
)

type instanceLease interface{ Close() error }

type apiServer interface {
	Run(context.Context) error
	Close(context.Context) error
}

type runDependencies struct {
	checkStart      func() error
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

func Run(ctx context.Context, localAppData string, recovery ...string) error {
	log, err := productlog.Open(localAppData, "agent")
	if err != nil {
		return fmt.Errorf("initialize diagnostics: %w", err)
	}
	dependencies := runDependencies{
		checkStart:   crashguard.CheckPortableRecovery,
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
	if len(recovery) != 0 {
		dependencies.checkStart = func() error {
			if err := crashguard.CheckPortableRecovery(); err != nil {
				return err
			}
			if !appsettings.RecoveryCurrent(localAppData, recovery[0]) {
				return errors.New("automatic recovery superseded")
			}
			return nil
		}
	}
	return run(ctx, localAppData, dependencies)
}

func run(ctx context.Context, localAppData string, dependencies runDependencies) (err error) {
	// This function owns the supplied logger, including failed startup paths.
	var instance instanceLease
	dependencies.log.Event("process_started")
	defer func() {
		// Drain logs before releasing agent ownership so an upgrade cannot race
		// the previous process's log writer.
		if err != nil {
			dependencies.log.Event("process_failed")
		} else {
			dependencies.log.Event("process_stopped")
		}
		dependencies.log.Close()
		if instance != nil {
			err = errors.Join(err, instance.Close())
		}
	}()
	if ctx == nil || localAppData == "" || dependencies.acquire == nil || dependencies.bootstrap == nil ||
		dependencies.startMonitor == nil || dependencies.listen == nil || dependencies.now == nil {
		return errors.New("resident agent dependencies and LOCALAPPDATA are required")
	}
	instance, err = dependencies.acquire()
	if err != nil {
		return err
	}
	if dependencies.checkStart != nil {
		if err := dependencies.checkStart(); err != nil {
			return err
		}
	}

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
	var identities *productidentity.Store
	if dependencies.productAdapters {
		traceRecorder := runtimestall.Start(lifecycle.Context(), localAppData, dependencies.log.Event)
		defer traceRecorder.Close()
		identities, err = productidentity.New(paths.Identity)
		if err != nil {
			return err
		}
		dependencies.pairing = &networkPairing{root: lifecycle.Context(), identity: identity, identities: identities, trust: trust}
		dependencies.runner = productRunnerWithTrace(identities, trust, dependencies.log, traceRecorder.Request, traceRecorder.RequestSample)
	}
	operations, err := New(lifecycle.Context(), identity.Public(), dependencies.runner, dependencies.pairing, trust)
	if err != nil {
		return err
	}
	operations.log = dependencies.log
	operations.localAppData = localAppData
	if dependencies.productAdapters {
		lifecycle.SetSessionTransitionHandler(operations.handleProtectedTransition)
		operations.preflight = func(alias string) error { _, err := prepareProductConnection(identities, trust, alias); return err }
		operations.credentials = identities
		updates, updateErr := peerupdate.Open(paths.Root, localapi.AgentGeneration)
		if updateErr == nil {
			updates.Observe = dependencies.log.Event
			removed, pruneErr := updates.Prune(localAppData)
			cleanupCode := "completed"
			if pruneErr != nil {
				cleanupCode = "failed"
				dependencies.log.Event("peer_stage_cleanup_failed")
			} else {
				dependencies.log.Event("peer_stage_cleanup_completed")
			}
			if productlog.SaveCleanup(localAppData, productlog.CleanupSummary{Time: time.Now().UTC().Format(time.RFC3339Nano), Code: cleanupCode, Removed: removed}) != nil {
				dependencies.log.Event("peer_stage_cleanup_failed")
			}
			operations.updates = updates
			updates.ReceiveDiagnostic = func(peer string, data []byte) (string, error) {
				return productlog.SaveRemoteSupport(localAppData, peer, data)
			}
			updates.ExportDiagnostic = operations.exportDiagnosticForRequest
			updates.ExportDiagnosticContext = operations.exportDiagnosticForRequestContext
			updates.ReceiveCollection = func(peer, id string, data []byte) (string, error) {
				return productlog.SaveRemoteSupportForRequest(localAppData, peer, id, data)
			}
			updates.OpenUI = operations.openUIForPeer
			updates.UIReady = operations.uiReadySince
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
