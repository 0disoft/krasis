//go:build linux

package residentagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productcli"
	"github.com/0disoft/krasis/internal/x11desktop"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// productRunner intentionally starts os.Executable as its recovery child. Only
// this opt-in test subprocess redirects that fixed child mode to the real agent
// built by the harness. Production dispatch and the crashguard pipe stay intact.
func init() {
	if os.Getenv("KRASIS_X11_TEST") == "1" && len(os.Args) == 2 && os.Args[1] == "product-crash-guard-internal" {
		if agent := os.Getenv("KRASIS_X11_TEST_AGENT"); filepath.IsAbs(agent) {
			if err := syscall.Exec(agent, []string{agent, os.Args[1]}, os.Environ()); err != nil {
				fmt.Fprintln(os.Stderr, "test crashguard exec:", err)
				os.Exit(1)
			}
		}
	}
}

// Run only against disposable X servers:
// KRASIS_X11_TEST=1 DISPLAY=:91 go test -count=1 -timeout=3m ./internal/residentagent -run '^TestLinuxX11CLIInteroperability$' -v
// The second display defaults to :92 (KRASIS_X11_TEST_OTHER_DISPLAY overrides it).
// Two isolated fixture processes bypass the production per-UID singleton, but
// use real authenticated local APIs, CLI pairing, productRunner, TLS transport,
// source hooks, injection and a separately built production crashguard child.
func TestLinuxX11CLIInteroperability(t *testing.T) {
	linuxX11Interoperability(t, false)
}

// Real agents additionally require two distinct UIDs because the singleton is
// intentionally per user, independent of HOME/XDG overrides. Provision users
// outside the test. Agents use real PAM/logind sessions via systemd-run;
// sudo -u alone would retain the caller's logind session with the wrong UID.
// Example: KRASIS_X11_TEST_LOCAL_USER=peerlinux2
// KRASIS_X11_TEST_PEER_USER=peerlinux KRASIS_X11_TEST=1 DISPLAY=:91 go test ...
func TestLinuxX11ActualCLIInteroperability(t *testing.T) {
	if os.Getenv("KRASIS_X11_TEST_PEER_USER") == "" {
		t.Skip("set KRASIS_X11_TEST_PEER_USER to an existing disposable local user; requires passwordless sudo")
	}
	linuxX11Interoperability(t, true)
}

type linuxX11Peer struct {
	flowNode
	cli, user          string
	env                []string
	unit               string
	expectedSystemStop *bool
}

type linuxX11End func(context.Context, linuxX11Peer, linuxX11Peer, *x11desktop.Desktop, *x11desktop.Desktop)

func linuxX11Interoperability(t *testing.T, actual bool, ending ...linuxX11End) {
	if os.Getenv("KRASIS_X11_TEST") != "1" {
		t.Skip("set KRASIS_X11_TEST=1 with two disposable Xvfb displays")
	}
	sourceDisplay := os.Getenv("DISPLAY")
	otherDisplay := os.Getenv("KRASIS_X11_TEST_OTHER_DISPLAY")
	if otherDisplay == "" {
		otherDisplay = ":92"
	}
	if !strings.HasPrefix(sourceDisplay, ":") || !strings.HasPrefix(otherDisplay, ":") || sourceDisplay == otherDisplay {
		t.Fatal("two distinct local disposable DISPLAY values are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	t.Cleanup(cancel)
	root := t.TempDir()
	if actual {
		// Both UIDs need to traverse only this binary directory. Identities and
		// control credentials live in separate 0700 directories owned by each UID.
		var err error
		root, err = os.MkdirTemp("", "krasis-x11-binaries-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) })
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Reject an unavailable display before building or starting any child.
	open := func(display string) *x11desktop.Desktop {
		t.Helper()
		t.Setenv("DISPLAY", display)
		d, err := x11desktop.Open(ctx)
		if err != nil {
			t.Fatalf("open %s: %v", display, err)
		}
		t.Cleanup(func() { _ = d.Close() })
		held, err := d.AnyPressed(ctx)
		if err != nil || held {
			t.Fatalf("%s must start with no pressed keys/buttons: held=%v err=%v", display, held, err)
		}
		return d
	}
	source, destination := open(sourceDisplay), open(otherDisplay)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = source.Key(cleanup, 0x1e, false)
		_ = source.Button(cleanup, 1, false)
		_ = destination.Key(cleanup, 0x1e, false)
		_ = destination.Button(cleanup, 1, false)
	})
	t.Setenv("DISPLAY", sourceDisplay)
	width := source.Dimensions().Width
	if err := source.SetPosition(ctx, width/2, source.Dimensions().Height/2); err != nil {
		t.Fatal(err)
	}
	if err := destination.SetPosition(ctx, destination.Dimensions().Width/2, destination.Dimensions().Height/2); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(root, "krasis-agent")
	build := exec.CommandContext(ctx, "go", "build", "-o", agent, "./cmd/krasis-agent")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real crashguard agent: %v\n%s", err, output)
	}
	var a, b linuxX11Peer
	if actual {
		firstUID, err := linuxX11UserCommand(ctx, os.Getenv("KRASIS_X11_TEST_LOCAL_USER"), "id", "-u").Output()
		if err != nil {
			t.Fatalf("resolve first disposable UID: %v", err)
		}
		secondUID, err := linuxX11UserCommand(ctx, os.Getenv("KRASIS_X11_TEST_PEER_USER"), "id", "-u").Output()
		if err != nil {
			t.Fatalf("resolve second disposable UID: %v", err)
		}
		if bytes.Equal(bytes.TrimSpace(firstUID), bytes.TrimSpace(secondUID)) {
			t.Fatal("actual agents require different UIDs")
		}
		cli := filepath.Join(root, "krasis-cli")
		build := exec.CommandContext(ctx, "go", "build", "-o", cli, "./cmd/krasis-cli")
		build.Dir = filepath.Join("..", "..")
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build actual CLI: %v\n%s", err, output)
		}
		a = linuxX11StartActualPeer(t, ctx, root, "a", sourceDisplay, agent, cli, os.Getenv("KRASIS_X11_TEST_LOCAL_USER"))
		b = linuxX11StartActualPeer(t, ctx, root, "b", otherDisplay, agent, cli, os.Getenv("KRASIS_X11_TEST_PEER_USER"))
	} else {
		a = linuxX11StartPeer(t, ctx, root, "a", sourceDisplay, agent)
		b = linuxX11StartPeer(t, ctx, root, "b", otherDisplay, agent)
	}
	t.Cleanup(func() {
		if t.Failed() {
			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			sx, sy, se := source.Position(cleanup)
			dx, dy, de := destination.Position(cleanup)
			t.Logf("final source=(%d,%d) %v destination=(%d,%d) %v", sx, sy, se, dx, dy, de)
			for name, node := range map[string]linuxX11Peer{"a": a, "b": b} {
				for _, action := range []string{"status", "diagnostics"} {
					data, stderr, code := linuxX11Call(cleanup, node, action)
					t.Logf("peer %s %s exit=%d %s %s", name, action, code, data, stderr)
				}
			}
		}
	})
	address := linuxX11FreeAddress(t)
	first := linuxX11CLI(t, ctx, a, "pair", "listen", "b", address, "--allow-inbound-control")
	second := linuxX11CLI(t, ctx, b, "pair", "dial", "a", address, "--allow-inbound-control")
	var confirmation string
	linuxX11Wait(t, ctx, "mutual pairing confirmation", func() bool {
		x := linuxX11CLI(t, ctx, a, "pair", "status", first.PairingID)
		y := linuxX11CLI(t, ctx, b, "pair", "status", second.PairingID)
		confirmation = x.Confirmation
		return confirmation != "" && confirmation == y.Confirmation
	})
	linuxX11CLI(t, ctx, a, "pair", "confirm", first.PairingID, confirmation)
	linuxX11CLI(t, ctx, b, "pair", "confirm", second.PairingID, confirmation)
	linuxX11Wait(t, ctx, "both identities committed", func() bool {
		return linuxX11CLI(t, ctx, a, "pair", "status", first.PairingID).Code == "pairing_completed" && linuxX11CLI(t, ctx, b, "pair", "status", second.PairingID).Code == "pairing_completed"
	})
	address = linuxX11FreeAddress(t)
	linuxX11CLI(t, ctx, a, "connection", "set", "b", address, "RIGHT")
	linuxX11CLI(t, ctx, b, "connection", "set", "a", address, "LEFT")
	linuxX11CLI(t, ctx, a, "connect", "b")
	linuxX11CLI(t, ctx, b, "connect", "a")
	linuxX11Wait(t, ctx, "authenticated product connection", func() bool {
		return linuxX11Status(t, ctx, a).State == "CONNECTED" && linuxX11Status(t, ctx, b).State == "CONNECTED"
	})
	if linuxX11Status(t, ctx, a).DeviceFingerprint == linuxX11Status(t, ctx, b).DeviceFingerprint {
		t.Fatal("peers unexpectedly share a product identity")
	}
	observer, err := xgb.NewConnDisplay(otherDisplay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Close)
	xroot := xproto.Setup(observer).DefaultScreen(observer).Root
	if err := xproto.ChangeWindowAttributesChecked(observer, xroot, xproto.CwEventMask, []uint32{xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease}).Check(); err != nil {
		t.Fatal(err)
	}
	// Entry must be caused by a physical-source event, never direct session calls.
	lastAttempt := time.Time{}
	linuxX11Wait(t, ctx, "cursor enters remote left edge", func() bool {
		x, _, e := destination.Position(ctx)
		sx, sy, se := source.Position(ctx)
		if e == nil && se == nil && x < 100 && sx == width/2 && sy == source.Dimensions().Height/2 {
			return true
		}
		// Once either side has entered its handoff position, do not inject
		// another edge warp while the other process finishes activation. That
		// would become real remote motion under slow PAM/VM scheduling.
		if !lastAttempt.IsZero() && (e == nil && x < 100 || se == nil && sx == width/2 && sy == source.Dimensions().Height/2) {
			return false
		}
		// CONNECTED precedes source readiness. Repeat a real center-to-edge
		// movement if the first movement occurred before capture started.
		if time.Since(lastAttempt) > 100*time.Millisecond {
			if err := source.SetPosition(ctx, width/2, source.Dimensions().Height/2); err != nil {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)
			if err := source.SetPosition(ctx, width-1, source.Dimensions().Height/2); err != nil {
				t.Fatal(err)
			}
			lastAttempt = time.Now()
		}
		return false
	})
	// The ordinary interoperability test covers motion, wheel and re-entry.
	// Lifecycle cases need one established handoff with held tokens only.
	if len(ending) == 0 {
		x, y, err := destination.Position(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := source.Move(ctx, 60, 15); err != nil {
			t.Fatal(err)
		}
		linuxX11Wait(t, ctx, "remote relative motion", func() bool {
			xx, yy, e := destination.Position(ctx)
			return e == nil && xx == x+60 && yy == y+15
		})
		seen := map[string]int{}
		drain := func() {
			t.Helper()
			for {
				event, e := observer.PollForEvent()
				if e != nil {
					t.Fatal(e)
				}
				if event == nil {
					return
				}
				switch v := event.(type) {
				case xproto.KeyPressEvent:
					seen["key-down"]++
				case xproto.KeyReleaseEvent:
					seen["key-up"]++
				case xproto.ButtonPressEvent:
					seen[fmt.Sprintf("button-%d-down", v.Detail)]++
				case xproto.ButtonReleaseEvent:
					seen[fmt.Sprintf("button-%d-up", v.Detail)]++
				}
			}
		}
		// Physical scan code 0x1e is A. Assert destination server state as well as
		// observed events; a transport acknowledgement alone does not prove injection.
		if err := source.Key(ctx, 0x1e, true); err != nil {
			t.Fatal(err)
		}
		linuxX11Wait(t, ctx, "remote keyboard down", func() bool {
			drain()
			held, e := destination.AnyPressed(ctx)
			return e == nil && held && seen["key-down"] > 0
		})
		if err := source.Key(ctx, 0x1e, false); err != nil {
			t.Fatal(err)
		}
		linuxX11Wait(t, ctx, "remote keyboard up", func() bool {
			drain()
			held, e := destination.AnyPressed(ctx)
			return e == nil && !held && seen["key-up"] > 0
		})
		if err := source.Button(ctx, 1, true); err != nil {
			t.Fatal(err)
		}
		linuxX11Wait(t, ctx, "remote mouse down", func() bool {
			drain()
			p, e := xproto.QueryPointer(observer, xroot).Reply()
			return e == nil && p.Mask&xproto.KeyButMaskButton1 != 0 && seen["button-1-down"] > 0
		})
		if err := source.Button(ctx, 1, false); err != nil {
			t.Fatal(err)
		}
		linuxX11Wait(t, ctx, "remote mouse up", func() bool {
			drain()
			p, e := xproto.QueryPointer(observer, xroot).Reply()
			return e == nil && p.Mask&xproto.KeyButMaskButton1 == 0 && seen["button-1-up"] > 0
		})
		for _, wheel := range []struct {
			dx, dy int32
			button int
		}{{0, 120, 4}, {0, -120, 5}, {-120, 0, 6}, {120, 0, 7}} {
			if err := source.Wheel(ctx, wheel.dx, wheel.dy); err != nil {
				t.Fatal(err)
			}
			linuxX11Wait(t, ctx, fmt.Sprintf("remote wheel %d", wheel.button), func() bool {
				drain()
				return seen[fmt.Sprintf("button-%d-down", wheel.button)] > 0 && seen[fmt.Sprintf("button-%d-up", wheel.button)] > 0
			})
		}
		// Returning across the left edge must ungrab the source and restore local
		// input. Poll actual grab ownership instead of a fixed handoff delay.
		if _, _, err := source.Move(ctx, -int32(x+61), 0); err != nil {
			t.Fatal(err)
		}
		linuxX11Wait(t, ctx, "return releases source grab", func() bool { return linuxX11CanGrab(sourceDisplay) })
		if err := source.SetPosition(ctx, width/2, source.Dimensions().Height/2); err != nil {
			t.Fatal(err)
		}
		// Inactive capture samples position every 4ms. Let it observe the rearm
		// position before the next edge warp; two immediate warps may coalesce.
		select {
		case <-time.After(30 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		// Exercise cancellation while two remote tokens are still held: cleanup
		// must release them and retire the real independent crashguard child.
		if err := source.SetPosition(ctx, width-1, source.Dimensions().Height/2); err != nil {
			t.Fatal(err)
		}
		linuxX11Wait(t, ctx, "second cursor entry", func() bool {
			xx, _, e := destination.Position(ctx)
			sx, sy, se := source.Position(ctx)
			return e == nil && se == nil && xx < 100 && sx == width/2 && sy == source.Dimensions().Height/2
		})
	}
	if err := source.Key(ctx, 0x1e, true); err != nil {
		t.Fatal(err)
	}
	if err := source.Button(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	linuxX11Wait(t, ctx, "held remote tokens before disconnect", func() bool {
		p, e := xproto.QueryPointer(observer, xroot).Reply()
		keys, ke := xproto.QueryKeymap(observer).Reply()
		if e != nil || ke != nil || p.Mask&xproto.KeyButMaskButton1 == 0 {
			return false
		}
		for _, bits := range keys.Keys {
			if bits != 0 {
				return true
			}
		}
		return false
	})
	if len(ending) != 0 {
		ending[0](ctx, a, b, source, destination)
		return
	}
	linuxX11CLI(t, ctx, a, "disconnect")
	// The source still owns the simulated physical holds; release those locally.
	_ = source.Key(ctx, 0x1e, false)
	_ = source.Button(ctx, 1, false)
	linuxX11Wait(t, ctx, "disconnect releases remote tokens and source grab", func() bool {
		held, e := destination.AnyPressed(ctx)
		return e == nil && !held && linuxX11CanGrab(sourceDisplay)
	})
	// Stop both peers explicitly: the final disconnect frame is best effort.
	// A peer that already consumed it may legitimately have no active session.
	data, stderr, exit := linuxX11Call(ctx, b, "disconnect")
	var stopped localapi.OperationResult
	if err := json.Unmarshal(data, &stopped); err != nil || (exit != 0 && stopped.Code != "no_active_session") {
		t.Fatalf("remote disconnect exit=%d decode=%v stdout=%s stderr=%s", exit, err, data, stderr)
	}
	linuxX11Wait(t, ctx, "both peers retire to local", func() bool {
		return linuxX11Status(t, ctx, a).State == "LOCAL" && linuxX11Status(t, ctx, b).State == "LOCAL"
	})
	t.Log("CLI mutual pairing, authenticated product sessions, entry/motion/key/button/four wheel directions/return and held-input cleanup passed")
}

// Each peer has its own process environment and X connection. It deliberately
// reuses the existing product fixture rather than weakening agent ownership.
func TestLinuxX11PeerHelper(t *testing.T) {
	root := os.Getenv("KRASIS_X11_TEST_PEER")
	if os.Getenv("KRASIS_X11_TEST") != "1" || root == "" {
		t.Skip("internal opt-in subprocess helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 105*time.Second)
	defer cancel()
	node := newFlowNode(t, ctx)
	runner := productRunner(node.pairing.identities, node.pairing.trust, nil)
	node.operations.runner = func(ctx context.Context, alias string, observer agent.SupervisorObserver) error {
		err := runner(ctx, alias, observer)
		t.Logf("product runner stopped: %v", err)
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "ready"), []byte(node.control), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "stop")); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("peer lifetime expired")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func linuxX11StartPeer(t *testing.T, ctx context.Context, root, name, display, agent string) linuxX11Peer {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filepath.Join(root, name+".log"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, executable, "-test.run=^TestLinuxX11PeerHelper$", "-test.v", "-test.timeout=110s")
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "DISPLAY=") && !strings.HasPrefix(value, "HOME=") && !strings.HasPrefix(value, "XDG_") && !strings.HasPrefix(value, "KRASIS_X11_TEST_PEER=") && !strings.HasPrefix(value, "KRASIS_X11_TEST_AGENT=") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "DISPLAY="+display, "HOME="+dir, "XDG_DATA_HOME="+dir, "XDG_CONFIG_HOME="+dir, "XDG_SESSION_TYPE=x11", "WAYLAND_DISPLAY=", "KRASIS_X11_TEST_PEER="+dir, "KRASIS_X11_TEST_AGENT="+agent)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		_ = output.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(dir, "stop"), nil, 0o600)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("peer %s: %v", name, err)
			}
		case <-time.After(8 * time.Second):
			_ = command.Process.Kill()
			<-done
			t.Errorf("peer %s failed to retire", name)
		}
		_ = output.Close()
		if t.Failed() {
			data, _ := os.ReadFile(filepath.Join(root, name+".log"))
			t.Logf("peer %s diagnostics:\n%s", name, data)
		}
	})
	var control string
	linuxX11Wait(t, ctx, "peer "+name+" API readiness", func() bool {
		data, e := os.ReadFile(filepath.Join(dir, "ready"))
		control = string(data)
		return e == nil && control != ""
	})
	return linuxX11Peer{flowNode: flowNode{control: control}}
}

func linuxX11Call(ctx context.Context, node linuxX11Peer, args ...string) ([]byte, []byte, int) {
	args = append([]string{"--json", "--control-dir", node.control}, args...)
	var stdout, stderr bytes.Buffer
	if node.cli != "" {
		command := linuxX11UserCommand(ctx, node.user, append(node.env, append([]string{node.cli}, args...)...)...)
		command.Stdout, command.Stderr = &stdout, &stderr
		if err := command.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				return stdout.Bytes(), stderr.Bytes(), exit.ExitCode()
			}
			fmt.Fprintln(&stderr, err)
			return stdout.Bytes(), stderr.Bytes(), -1
		}
		return stdout.Bytes(), stderr.Bytes(), 0
	}
	code := productcli.Run(ctx, args, &stdout, &stderr, "unused")
	return stdout.Bytes(), stderr.Bytes(), code
}

func linuxX11CLI(t *testing.T, ctx context.Context, node linuxX11Peer, args ...string) localapi.OperationResult {
	t.Helper()
	call, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	stdout, stderr, code := linuxX11Call(call, node, args...)
	if code != productcli.ExitOK {
		t.Fatalf("CLI %s exit=%d stdout=%s stderr=%s", args[0], code, stdout, stderr)
	}
	var result localapi.OperationResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func linuxX11Status(t *testing.T, ctx context.Context, node linuxX11Peer) localapi.Status {
	t.Helper()
	data, stderr, code := linuxX11Call(ctx, node, "status")
	var status localapi.Status
	if err := json.Unmarshal(data, &status); code != 0 || err != nil {
		t.Fatalf("status exit=%d decode=%v stderr=%s", code, err, stderr)
	}
	return status
}

// Always use argv, never a shell: users, paths and CLI values cannot become code.
func linuxX11UserCommand(ctx context.Context, user string, args ...string) *exec.Cmd {
	if user != "" {
		return exec.CommandContext(ctx, "sudo", append([]string{"-n", "-u", user, "--", "env", "-i", "PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"}, args...)...)
	}
	return exec.CommandContext(ctx, "env", append([]string{"-i", "PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"}, args...)...)
}

func linuxX11StartActualPeer(t *testing.T, ctx context.Context, root, name, display, agent, cli, user string) linuxX11Peer {
	t.Helper()
	output, err := linuxX11UserCommand(ctx, user, "mktemp", "-d", "/tmp/krasis-x11-data-XXXXXXXXXX").Output()
	if err != nil {
		t.Fatalf("create disposable data for %s: %v", user, err)
	}
	dir := strings.TrimSpace(string(output))
	if !filepath.IsAbs(dir) || !strings.HasPrefix(dir, "/tmp/krasis-x11-data-") || filepath.Clean(dir) != dir {
		t.Fatal("invalid temporary data path")
	}
	t.Cleanup(func() {
		if output, err := linuxX11UserCommand(context.Background(), user, "rm", "-rf", "--", dir).CombinedOutput(); err != nil {
			t.Errorf("remove disposable peer data: %v %s", err, output)
		}
	})
	peer := linuxX11Peer{flowNode: flowNode{control: filepath.Join(dir, "Krasis", "control")}, cli: cli, user: user, env: []string{"DISPLAY=" + display, "HOME=" + dir, "XDG_DATA_HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "XDG_SESSION_TYPE=x11"}}
	// Each UID gets only its own display cookie in its private data directory.
	// Never relax X server access control or share the test runner's authority file.
	if authority := os.Getenv("XAUTHORITY"); authority != "" {
		extract := exec.CommandContext(ctx, "xauth", "-f", authority, "nextract", "-", display)
		cookie, err := extract.Output()
		if err != nil || len(bytes.TrimSpace(cookie)) == 0 {
			t.Fatal("extract private X11 authority for peer", err)
		}
		peerAuthority := filepath.Join(dir, "Xauthority")
		merge := linuxX11UserCommand(ctx, user, "xauth", "-f", peerAuthority, "nmerge", "-")
		merge.Stdin = bytes.NewReader(cookie)
		if err := merge.Run(); err != nil {
			t.Fatal("install private X11 authority for peer", err)
		}
		peer.env = append(peer.env, "XAUTHORITY="+peerAuthority)
	}
	return linuxX11LaunchActualPeer(t, ctx, root, name, agent, peer)
}

func linuxX11LaunchActualPeer(t *testing.T, ctx context.Context, root, name, agent string, peer linuxX11Peer) linuxX11Peer {
	t.Helper()
	unit := filepath.Base(root) + "-" + name + ".service"
	peer.unit = unit
	peer.expectedSystemStop = new(bool)
	serviceUser := peer.user
	if serviceUser == "" {
		serviceUser = fmt.Sprint(os.Geteuid())
	}
	// PAM supplies the authoritative XDG_SESSION_ID to this service. Preserve
	// that generated environment, rather than inheriting the SSH caller's ID.
	pamService := os.Getenv("KRASIS_X11_TEST_PAM_SERVICE")
	if pamService == "" {
		pamService = "login"
	}
	for _, ch := range pamService {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			t.Fatal("invalid test PAM service name")
		}
	}
	// These private Xvfb sessions have no desktop locker. PAM may export a
	// user-manager bus; do not mistake it for an actual graphical desktop bus.
	args := []string{"-n", "systemd-run", "--quiet", "--collect", "--wait", "--pipe", "--unit=" + unit, "--uid=" + serviceUser, "--property=PAMName=" + pamService, "--", "env", "-u", "DBUS_SESSION_BUS_ADDRESS"}
	args = append(args, peer.env...)
	command := exec.CommandContext(ctx, "sudo", append(args, agent)...)
	logPath := filepath.Join(root, name+".log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		// Stop the service owner, not only systemd-run's waiting client.
		stop, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = exec.CommandContext(stop, "sudo", "-n", "systemctl", "kill", "--kill-whom=main", "--signal=SIGINT", unit).Run()
		select {
		case err := <-done:
			if err != nil && !*peer.expectedSystemStop {
				t.Errorf("actual peer %s exit: %v", name, err)
			}
		case <-time.After(8 * time.Second):
			_ = exec.Command("sudo", "-n", "systemctl", "kill", "--kill-whom=all", "--signal=SIGKILL", unit).Run()
			_ = command.Process.Kill()
			<-done
			t.Errorf("actual peer %s failed to retire", name)
		}
		_ = log.Close()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Logf("actual peer %s diagnostics:\n%s", name, data)
			// Read as the owning UID; do not relax identity/control permissions.
			data, _ = linuxX11UserCommand(context.Background(), peer.user, "find", filepath.Join(filepath.Dir(peer.control), "diagnostics"), "-maxdepth", "1", "-name", "agent.jsonl", "-exec", "cat", "{}", "+").CombinedOutput()
			t.Logf("actual peer %s product log:\n%s", name, data)
		}
	})
	linuxX11Wait(t, ctx, "actual peer "+name+" readiness", func() bool { _, _, code := linuxX11Call(ctx, peer, "status"); return code == 0 })
	return peer
}

func linuxX11Wait(t *testing.T, ctx context.Context, description string, check func() bool) {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s: %v", description, ctx.Err())
		case <-deadline.C:
			t.Fatalf("timed out: %s", description)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func linuxX11FreeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func linuxX11CanGrab(display string) bool {
	conn, err := xgb.NewConnDisplay(display)
	if err != nil {
		return false
	}
	defer conn.Close()
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	pointer, err := xproto.GrabPointer(conn, false, root, 0, xproto.GrabModeAsync, xproto.GrabModeAsync, 0, 0, 0).Reply()
	if err != nil || pointer.Status != xproto.GrabStatusSuccess {
		return false
	}
	keyboard, err := xproto.GrabKeyboard(conn, false, root, 0, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
	return err == nil && keyboard.Status == xproto.GrabStatusSuccess
}
