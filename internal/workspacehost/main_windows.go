//go:build windows && desktop

package workspacehost

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	windowsassets "github.com/0disoft/krasis/assets/windows"
	"github.com/0disoft/krasis/frontend"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/wininput"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// WorkspaceSmoke exists only in the bounded hidden-window check. It accepts no
// commands and proves the renderer received a validated, real agent snapshot.
type WorkspaceSmoke struct{ result chan bool }

func (s *WorkspaceSmoke) Report(schema, state, code string) {
	ok := schema == productpanel.BridgeSchema && code == "ok"
	switch state {
	case "LOCAL", "CONNECTING", "CONNECTED", "RECONNECTING", "DISCONNECTING", "FAULTED":
	default:
		ok = false
	}
	select {
	case s.result <- ok:
	default:
	}
}

func Run() {
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
	if len(os.Args) != 1 && !smoke {
		updated, err = productpanel.ParseWorkspaceUpdateLaunch(os.Args[1:])
		if err != nil {
			os.Exit(1)
		}
	}
	assets, err := fs.Sub(frontend.NativeAssets, "dist-native")
	if err != nil {
		os.Exit(1)
	}
	service := &WorkspaceService{raw: bridge, ctx: ctx, control: paths.Control, title: fmt.Sprintf("Krasis · g%d", localapi.AgentGeneration)}
	if !smoke {
		startup, startupErr := productpanel.NewStartup(paths.Control)
		if startupErr != nil {
			os.Exit(1)
		}
		if updated.Revision != "" {
			updateCtx, done := context.WithTimeout(ctx, 40*time.Second)
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
		base := os.Getenv("LOCALAPPDATA")
		self, executableErr := os.Executable()
		if executableErr != nil {
			os.Exit(1)
		}
		service.updates = productpanel.NewUpdateTransferBridge(ctx, func() (productpanel.UpdateSettingsClient, error) { return localapi.OpenClient(paths.Control) }, updateBundleDirectory(self))
		service.receiver = productpanel.NewWorkspaceUpdateReceiver(ctx, func() (productpanel.UpdateSettingsClient, error) { return localapi.OpenClient(paths.Control) }, productpanel.LaunchReceivedWorkspaceUpdate)
		service.diagnostics = productpanel.NewDiagnosticsBridge(ctx,
			func() (productpanel.DiagnosticClient, error) { return localapi.OpenClient(paths.Control) },
			func() error { _, err := productlog.Export(base); return err },
			func() (bool, bool, error) { return diagnosticFiles(base) },
			func() error { return openDiagnosticsFolder(base) })
	}
	service.pairs = productpanel.NewPairingBridge(ctx, func() (productpanel.PairingClient, error) {
		return localapi.OpenClient(paths.Control)
	}, nil)
	services := []application.Service{application.NewService(service)}
	check := &WorkspaceSmoke{result: make(chan bool, 1)}
	if smoke {
		services = append(services, application.NewService(check))
	}
	var window *application.WebviewWindow
	instance := &application.SingleInstanceOptions{
		UniqueID: "0disoft-krasis-workspace-preview-v1",
		OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			if window != nil {
				window.Restore()
				window.Show()
				window.Focus()
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
		Name:           "Krasis",
		Icon:           windowsassets.Icon,
		OnShutdown:     cancel,
		MarshalError:   func(error) []byte { return []byte(`"bridge_failed"`) },
		Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Services:       services,
		SingleInstance: instance,
		Windows:        application.WindowsOptions{WebviewUserDataPath: webviewPath},
	})
	service.quit = app.Quit
	url := "/"
	if smoke {
		url = "/?smoke=1"
	}
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: service.title,
		Width: 1180, Height: 850, MinWidth: 720, MinHeight: 620,
		BackgroundColour: application.NewRGB(24, 27, 33),
		URL:              url,
		Hidden:           smoke && !visibleSmoke,
	})
	shown := make(chan bool, 1)
	if visibleSmoke || (!smoke && updated.Revision != "") {
		var showOnce sync.Once
		window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
			showOnce.Do(func() {
				// The first ShowWindow can consume an older sender's SW_HIDE,
				// even at runtime-ready. The next call uses our explicit SW_SHOW.
				window.Show()
				window.Show()
				window.Restore()
				window.Focus()
				shown <- window.IsVisible()
			})
		})
	}
	if !smoke {
		tray := app.SystemTray.New()
		tray.SetIcon(windowsassets.Icon)
		tray.SetTooltip("Krasis")
		tray.SetLabel("Krasis")
		menu := app.NewMenu()
		menu.Add("열기 / Open").OnClick(func(*application.Context) { window.Show(); window.Restore(); window.Focus() })
		menu.Add("연결 해제 / Disconnect").OnClick(func(*application.Context) {
			service.disconnectAsync()
		})
		menu.Add("PIN 창 입력 시험").OnClick(func(*application.Context) {
			if err := launchCredentialProbe(paths.Control); err != nil {
				showCredentialProbeLaunchError()
			}
		})
		menu.AddSeparator()
		menu.Add("관리 앱 종료 · 에이전트 유지 / Quit UI").OnClick(func(*application.Context) { app.Quit() })
		tray.SetMenu(menu)
		window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { window.Hide(); e.Cancel() })
	}
	done := make(chan bool, 1)
	var rendererOK, windowVisible bool
	if smoke {
		go func() {
			timer := time.NewTimer(25 * time.Second)
			defer timer.Stop()
			ok := false
			select {
			case ok = <-check.result:
			case <-timer.C:
			case <-ctx.Done():
			}
			rendererOK = ok
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
			done <- ok
			app.Quit()
		}()
	}
	err = app.Run()
	if smoke {
		cancel()
		ok := <-done
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			RendererAgentRoundtrip bool `json:"rendererAgentRoundtrip"`
			UpdatedWindowVisible   bool `json:"updatedWindowVisible,omitempty"`
		}{rendererOK, visibleSmoke && windowVisible})
		if !ok {
			os.Exit(2)
		}
	}
	if err != nil {
		os.Exit(1)
	}
}
