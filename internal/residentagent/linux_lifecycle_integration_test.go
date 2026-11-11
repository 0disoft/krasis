//go:build linux

package residentagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/x11desktop"
)

// Opt-in: dedicated unprivileged users, sudo, systemd/logind, and disposable
// X servers are required. Never terminate the test runner's own login session.
func TestLinuxX11ActualLifecycle(t *testing.T) {
	if os.Getenv("KRASIS_X11_TEST") != "1" || os.Getenv("KRASIS_X11_TEST_LOCAL_USER") == "" || os.Getenv("KRASIS_X11_TEST_PEER_USER") == "" {
		t.Skip("requires two disposable UIDs, real PAM/logind, sudo and isolated X11 displays")
	}
	for _, action := range []string{"lock-session", "lock-hint", "terminate-session"} {
		for _, role := range []string{"source", "receiver"} {
			t.Run(action+"/"+role, func(t *testing.T) {
				linuxX11Interoperability(t, true, func(ctx context.Context, a, b linuxX11Peer, source, destination *x11desktop.Desktop) {
					target, survivor := a, b
					if role == "receiver" {
						target, survivor = b, a
					}
					session := linuxX11ActualSession(t, ctx, target)
					if action == "terminate-session" {
						// logind may stop the transient service with a signal. The
						// native state and peer cleanup assertions remain mandatory.
						*target.expectedSystemStop = true
					}
					if action == "lock-hint" {
						linuxX11SetLockHint(t, ctx, target, session)
					} else {
						command := exec.CommandContext(ctx, "sudo", "-n", "loginctl", action, session)
						if output, err := command.CombinedOutput(); err != nil {
							t.Fatalf("real logind %s: %v %s", action, err, output)
						}
					}
					// Keep physical source tokens held until recovery is proven;
					// an ordinary forwarded key-up must not make this test pass.
					linuxX11Wait(t, ctx, "lifecycle releases remote holds and source grabs", func() bool {
						held, err := destination.AnyPressed(ctx)
						return err == nil && !held && linuxX11CanGrab(os.Getenv("DISPLAY"))
					})
					if err := source.Key(ctx, 0x1e, false); err != nil {
						t.Fatal(err)
					}
					if err := source.Button(ctx, 1, false); err != nil {
						t.Fatal(err)
					}
					linuxX11Wait(t, ctx, "unsafe session stops its local API", func() bool {
						_, _, code := linuxX11Call(ctx, target, "status")
						return code != 0
					})
					// Prove the unaffected UID remains usable after the event.
					linuxX11Call(ctx, survivor, "disconnect")
					linuxX11Wait(t, ctx, "surviving agent returns LOCAL", func() bool {
						return linuxX11Status(t, ctx, survivor).State == "LOCAL"
					})
					// Start a new PAM session with the SAME identity/trust/data.
					// Recovery must not require re-pairing or deleting credentials.
					root := filepath.Dir(target.cli)
					restarted := linuxX11LaunchActualPeer(t, ctx, root, "restarted", filepath.Join(root, "krasis-agent"), target)
					if next := linuxX11ActualSession(t, ctx, restarted); next == session {
						t.Fatal("restart reused the unsafe login session")
					}
					alias, survivorAlias := "b", "a"
					if role == "receiver" {
						alias, survivorAlias = "a", "b"
					}
					linuxX11CLI(t, ctx, restarted, "connect", alias)
					linuxX11CLI(t, ctx, survivor, "connect", survivorAlias)
					linuxX11Wait(t, ctx, "same identities reconnect after lifecycle stop", func() bool {
						return linuxX11Status(t, ctx, restarted).State == "CONNECTED" && linuxX11Status(t, ctx, survivor).State == "CONNECTED"
					})
					linuxX11CLI(t, ctx, restarted, "disconnect")
					t.Logf("real PAM/logind %s on %s: held input and grab released, unsafe API stopped, same identities reconnect in a new PAM session", action, role)
				})
			})
		}
	}
}

func linuxX11SetLockHint(t *testing.T, ctx context.Context, peer linuxX11Peer, session string) {
	t.Helper()
	const service = "org.freedesktop.login1"
	data, err := linuxX11UserCommand(ctx, peer.user, "busctl", "--system", "call", service, "/org/freedesktop/login1", service+".Manager", "GetSession", "s", session).Output()
	parts := strings.Fields(string(data))
	if err != nil || len(parts) != 2 || parts[0] != "o" {
		t.Fatal("resolve real logind object path", err)
	}
	path, err := strconv.Unquote(parts[1])
	if err != nil || !strings.HasPrefix(path, "/org/freedesktop/login1/session/") {
		t.Fatal("invalid real logind object path", err)
	}
	command := linuxX11UserCommand(ctx, peer.user, "busctl", "--system", "call", service, path, service+".Session", "SetLockedHint", "b", "true")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("real logind LockedHint: %v %s", err, output)
	}
}

func linuxX11ActualSession(t *testing.T, ctx context.Context, peer linuxX11Peer) string {
	t.Helper()
	pidText, err := exec.CommandContext(ctx, "sudo", "-n", "systemctl", "show", peer.unit, "--property=MainPID", "--value").Output()
	pid, parseErr := strconv.ParseUint(strings.TrimSpace(string(pidText)), 10, 32)
	if err != nil || parseErr != nil || pid < 2 {
		t.Fatal("resolve actual agent PID", err, parseErr)
	}
	// Capture only this disposable service's environment and never log it.
	data, err := linuxX11UserCommand(ctx, peer.user, "cat", fmt.Sprintf("/proc/%d/environ", pid)).Output()
	if err != nil {
		t.Fatal("read disposable agent session metadata", err)
	}
	var session string
	for _, item := range strings.Split(string(data), "\x00") {
		if value, ok := strings.CutPrefix(item, "XDG_SESSION_ID="); ok {
			session = value
		}
	}
	if session == "" || len(session) > 128 || strings.ContainsAny(session, "/\\\x00\r\n\t ") {
		t.Fatal("agent has no safe PAM session ID")
	}
	uid, err := linuxX11UserCommand(ctx, peer.user, "id", "-u").Output()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := exec.CommandContext(ctx, "loginctl", "show-session", session, "--property=User", "--value").Output()
	if err != nil || strings.TrimSpace(string(owner)) != strings.TrimSpace(string(uid)) {
		t.Fatal("refusing lifecycle action on a foreign login session", err)
	}
	// Protect the test runner even if a fixture accidentally inherits its ID.
	if session == os.Getenv("XDG_SESSION_ID") {
		t.Fatal("refusing lifecycle action on the test runner session")
	}
	t.Logf("validated disposable session %s for UID %s", session, strings.TrimSpace(string(uid)))
	return session
}
