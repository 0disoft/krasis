//go:build windows && desktop

package workspacehost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	windowsassets "github.com/0disoft/krasis/assets/windows"
	"github.com/0disoft/krasis/frontend"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/buildinfo"
	"github.com/0disoft/krasis/internal/launcherui"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productcli"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/releaseupdate"
	"github.com/0disoft/krasis/internal/wininput"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// WorkspaceSmoke exists only in the bounded hidden-window check. It accepts no
// commands and proves the renderer received a validated, real agent snapshot.
type workspaceSmokeResult struct {
	Roundtrip bool
	Healthy   bool
	State     string
	Code      string
}
type WorkspaceSmoke struct{ result chan workspaceSmokeResult }

func (s *WorkspaceSmoke) Report(schema, state, code string) {
	ok := schema == productpanel.BridgeSchema && (code == "ok" || code == "recovery_required" && state == "FAULTED")
	switch state {
	case "LOCAL", "CONNECTING", "CONNECTED", "RECONNECTING", "DISCONNECTING", "FAULTED":
	default:
		ok = false
	}
	result := workspaceSmokeResult{Roundtrip: ok, Healthy: ok && code == "ok" && state != "FAULTED", State: "UNKNOWN", Code: "unavailable"}
	if ok {
		result.State, result.Code = state, code
	}
	select {
	case s.result <- result:
	default:
	}
}

func Run() {
	if len(os.Args) == 2 && os.Args[1] == appsettings.RecoveryArgument {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		code := productcli.Run(ctx, []string{"app", "ensure", "--json"}, io.Discard, io.Discard, os.Getenv("LOCALAPPDATA"))
		cancel()
		os.Exit(code)
	}
	if len(os.Args) == 3 && os.Args[1] == "--release-key-check" {
		if !releaseupdate.MatchEmbeddedKey(os.Args[2]) {
			os.Exit(2)
		}
		return
	}
	paths, err := productidentity.ResolveWindowsPaths(os.Getenv("LOCALAPPDATA"))
	if err != nil {
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bridge := productpanel.NewDesktopBridge(ctx, func() (productpanel.Client, error) {
		return localapi.OpenClient(paths.Control)
	})
	// Read-only diagnostic mode never starts a window or changes agent state.
	if len(os.Args) == 2 && os.Args[1] == "--probe" {
		value := bridge.Refresh()
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			State     string `json:"state"`
			PeerCount int    `json:"peerCount"`
			Code      string `json:"code"`
		}{value.State, len(value.Peers), value.Code})
		if value.Code != "ok" {
			os.Exit(2)
		}
		return
	}
	visibleSmoke := len(os.Args) == 2 && os.Args[1] == "--smoke-update-window"
	smoke := visibleSmoke || (len(os.Args) == 2 && os.Args[1] == "--smoke")
	var updated productpanel.WorkspaceUpdateLaunch
	startAtLogin := len(os.Args) == 2 && os.Args[1] == "--startup"
	if len(os.Args) != 1 && !smoke && !startAtLogin {
		updated, err = productpanel.ParseWorkspaceUpdateLaunch(os.Args[1:])
		if err != nil {
			os.Exit(1)
		}
	}
	if !smoke {
		inJob, jobErr := launchrecord.InJob()
		if jobErr != nil {
			os.Exit(1)
		}
		if inJob && os.Getenv("KRASIS_INDEPENDENT_LAUNCH") != "1" {
			self, err := os.Executable()
			if err != nil {
				os.Exit(1)
			}
			command := exec.Command(self, os.Args[1:]...)
			command.Dir = filepath.Dir(self)
			if err = launchrecord.StartIndependent(command); err != nil {
				os.Exit(1)
			}
			return
		}
	}
	assets, err := fs.Sub(frontend.NativeAssets, "dist-native")
	if err != nil {
		os.Exit(1)
	}
	service := &WorkspaceService{raw: bridge, ctx: ctx, control: paths.Control, title: fmt.Sprintf("Krasis · g%d", localapi.AgentGeneration)}
	service.clipboardWake = make(chan struct{}, 1)
	service.startAtLogin = startAtLogin
	if !smoke {
		startup, startupErr := productpanel.NewStartup(paths.Control)
		if startupErr != nil {
			os.Exit(1)
		}
		service.inputRefresh = startup.RefreshProtectedInput
		service.inputSetup = startup.InputSetup
		if updated.Revision != "" {
			updateCtx, done := context.WithTimeout(ctx, 3*time.Minute)
			err = prepareUpdatedWorkspace(updateCtx, paths.Control, startup, updated)
			done()
			if err != nil {
				os.Exit(1)
			}
		}
		var lastGeometry wininput.DisplayGeometry
		var stableSince time.Time
		displayReady := func() bool {
			geometry := wininput.CurrentDisplayGeometry()
			if !geometry.Valid() {
				stableSince = time.Time{}
				return false
			}
			if geometry != lastGeometry {
				lastGeometry, stableSince = geometry, time.Now()
				return false
			}
			return time.Since(stableSince) >= 2*time.Second
		}
		service.managed = productpanel.NewDesktopLifecycle(ctx, func() (productpanel.Client, error) { return localapi.OpenClient(paths.Control) }, startup.Ensure, diagnose(os.Getenv("LOCALAPPDATA"), paths.Control), displayReady)
		service.managed.StartupResume = service.resumeAtStartup
		service.managed.StartupObservedReady = startup.ObservedReady
		service.managed.PendingInputSetup = startup.InputSetup
		service.managed.PendingInputRestore = startup.RestoreProtectedInput
		if err := service.managed.SetUpdatedWorkspaceResume(updated); err != nil {
			os.Exit(1)
		}
		service.managed.StartupMaintenanceNeeded = startup.LaunchMaintenanceNeeded
		service.managed.StartupMaintenance = startup.RefreshProtectedInputAtLaunch
		base := os.Getenv("LOCALAPPDATA")
		self, executableErr := os.Executable()
		if executableErr != nil {
			os.Exit(1)
		}
		service.prepareRecovery = func() error {
			return launchrecord.Register(base, self, localapi.AgentGeneration, buildinfo.Read().Revision)
		}
		service.prepareStartMenu = func() error { return launchrecord.RegisterStartMenu(base) }
		service.prepareSettings = func() error {
			if err := appsettings.PauseRecovery(base, false); err != nil {
				return err
			}
			return service.prepareAppSettings()
		}
		service.updates = productpanel.NewUpdateTransferBridge(ctx, func() (productpanel.UpdateSettingsClient, error) { return localapi.OpenClient(paths.Control) }, updateBundleDirectory(self), transitionBundleDirectory(self), probeTransitionBundleDirectory(self), installerTransitionBundleDirectory(self))
		service.receiver = productpanel.NewWorkspaceUpdateReceiver(ctx, func() (productpanel.UpdateSettingsClient, error) { return localapi.OpenClient(paths.Control) }, productpanel.LaunchRequestedWorkspaceUpdate)
		service.diagnostics = productpanel.NewDiagnosticsBridge(ctx,
			func() (productpanel.DiagnosticClient, error) { return localapi.OpenClient(paths.Control) },
			func() error { _, err := productlog.Export(base); return err },
			func() (bool, bool, error) { return diagnosticFiles(base) },
			func() error { return openDiagnosticsFolder(base) })
		service.diagnostics.ConfigureCollectionStorage(
			func() (*productlog.CollectionReceipt, error) { return productlog.LoadCollectionReceipt(base) },
			func(r productlog.CollectionReceipt) error { return productlog.SaveCollectionReceipt(base, r) })
	}
	service.pairs = productpanel.NewPairingBridge(ctx, func() (productpanel.PairingClient, error) {
		return localapi.OpenClient(paths.Control)
	}, nil)
	services := []application.Service{application.NewService(service)}
	check := &WorkspaceSmoke{result: make(chan workspaceSmokeResult, 1)}
	if smoke {
		services = append(services, application.NewService(check))
	}
	var window *application.WebviewWindow
	instance := &application.SingleInstanceOptions{
		UniqueID:       "0disoft-krasis-workspace-preview-v1",
		AdditionalData: map[string]string{"generation": strconv.Itoa(localapi.AgentGeneration), "revision": buildinfo.Read().Revision},
		OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
			service.requestUIReplacement(data.Args, data.AdditionalData)
			if window != nil {
				window.Restore()
				fitWorkspaceWindow(window, false)
				window.Show()
				window.Focus()
				if window.IsVisible() {
					service.reportUIReady()
				}
			}
		},
	}
	if smoke {
		instance = nil
	}
	webviewPath := filepath.Join(paths.Root, "workspace-webview")
	if smoke {
		webviewPath, err = filepath.Abs(".cache/workspace-webview")
		if err != nil {
			os.Exit(1)
		}
	}
	app := application.New(application.Options{
		Name: "Krasis",
		Icon: windowsassets.Icon,
		OnShutdown: func() {
			if hwnd := service.clipboardWindow.Swap(0); hwnd != 0 {
				launcherui.ListenClipboardChanges(hwnd, false)
			}
			cancel()
		},
		MarshalError:   func(error) []byte { return []byte(`"bridge_failed"`) },
		Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Services:       services,
		SingleInstance: instance,
		Windows: application.WindowsOptions{
			WebviewUserDataPath: webviewPath,
			WndProcInterceptor: func(hwnd uintptr, msg uint32, _, _ uintptr) (uintptr, bool) {
				if msg == launcherui.ClipboardUpdateMessage && hwnd == service.clipboardWindow.Load() {
					launcherui.WakeClipboard(service.clipboardWake)
				}
				return 0, false
			},
		},
	})
	// application.New has already admitted this process as the primary instance.
	// Service startup prepares recovery independently of renderer-ready events.
	if !smoke {
		service.log, _ = productlog.Open(os.Getenv("LOCALAPPDATA"), "control")
	}
	service.quit = app.Quit
	url := "/"
	if smoke {
		url = "/?smoke=1"
	}
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     service.title,
		Frameless: true,
		Width:     1000, Height: 640, MinWidth: 720, MinHeight: 480,
		BackgroundColour: application.NewRGB(24, 27, 33),
		URL:              url,
		// Fit to the actual monitor before the first Show, including startup
		// launches whose window remains hidden in the tray.
		Hidden: true,
	})
	service.visibleWindow.Store(window)
	var initialWindowFit sync.Once
	fitInitialWindow := func() { initialWindowFit.Do(func() { fitWorkspaceWindow(window, true) }) }
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		fitInitialWindow()
	})
	if !smoke {
		window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
			hwnd := uintptr(window.NativeWindow())
			previous := service.clipboardWindow.Swap(hwnd)
			if previous != hwnd {
				if previous != 0 {
					launcherui.ListenClipboardChanges(previous, false)
				}
				launcherui.ListenClipboardChanges(hwnd, true)
				launcherui.WakeClipboard(service.clipboardWake)
			}
		})
	}
	shown := make(chan bool, 1)
	if visibleSmoke || (!smoke && !startAtLogin) {
		var showOnce sync.Once
		window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
			showOnce.Do(func() {
				window.Restore()
				fitInitialWindow()
				// The first ShowWindow can consume an older sender's SW_HIDE,
				// even at runtime-ready. The next call uses our explicit SW_SHOW.
				window.Show()
				window.Show()
				window.Focus()
				shown <- window.IsVisible()
				if !smoke && window.IsVisible() {
					service.reportUIReady()
				}
			})
		})
	}
	if !smoke {
		tray := app.SystemTray.New()
		tray.SetIcon(windowsassets.Icon)
		tray.SetTooltip("Krasis")
		tray.SetLabel("Krasis")
		menu := app.NewMenu()
		labels, _ := trayMenuLabels("ko")
		openItem := menu.Add(labels[0]).OnClick(func(*application.Context) {
			window.Restore()
			fitWorkspaceWindow(window, false)
			window.Show()
			window.Focus()
		})
		disconnectItem := menu.Add(labels[1]).OnClick(func(*application.Context) {
			service.disconnectAsync()
		})
		probeItem := menu.Add(labels[2]).OnClick(func(*application.Context) {
			if err := launchCredentialProbe(paths.Control); err != nil {
				showCredentialProbeLaunchError()
			}
		})
		menu.AddSeparator()
		quitItem := menu.Add(labels[3]).OnClick(func(*application.Context) { app.Quit() })
		service.updateTrayLabels = func(labels [4]string) {
			openItem.SetLabel(labels[0])
			disconnectItem.SetLabel(labels[1])
			probeItem.SetLabel(labels[2])
			quitItem.SetLabel(labels[3])
		}
		tray.SetMenu(menu)
		window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { window.Hide(); e.Cancel() })
	}
	done := make(chan bool, 1)
	var windowVisible, windowFitsWorkArea bool
	observed := workspaceSmokeResult{State: "UNKNOWN", Code: "unavailable"}
	if smoke {
		go func() {
			timer := time.NewTimer(25 * time.Second)
			defer timer.Stop()
			ok := false
			select {
			case observed = <-check.result:
				ok = observed.Roundtrip
			case <-timer.C:
			case <-ctx.Done():
			}
			if ok {
				fitInitialWindow()
				if screen, screenErr := window.GetScreen(); screenErr == nil && screen != nil {
					bounds, work := window.Bounds(), screen.WorkArea
					windowFitsWorkArea = bounds.Width > 0 && bounds.Height > 0 && bounds.X >= work.X && bounds.Y >= work.Y &&
						bounds.X+bounds.Width <= work.X+work.Width && bounds.Y+bounds.Height <= work.Y+work.Height
				}
			}
			if visibleSmoke && ok {
				select {
				case ok = <-shown:
					windowVisible = ok
				case <-timer.C:
					ok = false
				case <-ctx.Done():
					ok = false
				}
			}
			done <- ok && observed.Healthy && windowFitsWorkArea
			app.Quit()
		}()
	}
	err = app.Run()
	if smoke {
		cancel()
		ok := <-done
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			RendererAgentRoundtrip bool   `json:"rendererAgentRoundtrip"`
			AgentHealthy           bool   `json:"agentHealthy"`
			AgentState             string `json:"agentState"`
			AgentCode              string `json:"agentCode"`
			WindowFitsWorkArea     bool   `json:"windowFitsWorkArea"`
			UpdatedWindowVisible   bool   `json:"updatedWindowVisible,omitempty"`
		}{observed.Roundtrip, observed.Healthy, observed.State, observed.Code, windowFitsWorkArea, visibleSmoke && windowVisible})
		if !ok {
			os.Exit(2)
		}
	}
	if err != nil {
		os.Exit(1)
	}
}
