//go:build windows && desktop

package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/frontend"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/wailsapp/wails/v3/pkg/application"
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

func main() {
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
	smoke := len(os.Args) == 2 && os.Args[1] == "--smoke"
	if len(os.Args) != 1 && !smoke {
		os.Exit(1)
	}
	assets, err := fs.Sub(frontend.NativeAssets, "dist-native")
	if err != nil {
		os.Exit(1)
	}
	services := []application.Service{application.NewService(bridge)}
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
		Name:           "Krasis Workspace",
		OnShutdown:     cancel,
		MarshalError:   func(error) []byte { return []byte(`"bridge_failed"`) },
		Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Services:       services,
		SingleInstance: instance,
		Windows:        application.WindowsOptions{WebviewUserDataPath: webviewPath},
	})
	url := "/"
	if smoke {
		url = "/?smoke=1"
	}
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Krasis Workspace · 연결 테스트 · " + productpanel.CurrentBuildLabel(),
		Width: 1180, Height: 850, MinWidth: 720, MinHeight: 620,
		BackgroundColour: application.NewRGB(24, 27, 33),
		URL:              url,
		Hidden:           smoke,
	})
	done := make(chan bool, 1)
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
		}{ok})
		if !ok {
			os.Exit(2)
		}
	}
	if err != nil {
		os.Exit(1)
	}
}
