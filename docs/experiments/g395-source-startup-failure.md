# g395: preserve source startup cleanup failure

A valid, authenticated source retirement with failed status was converted to
ErrProtocol during startup, whereas the live reader preserves ErrRecovery for
the same response. Startup now retains ErrRecovery, without granting readiness,
clean retirement or a retry. Wrong binding/sequence and malformed frames still
fail as protocol errors.

Startup diagnostics also retain a fixed outcome category alongside the existing
stage and Win32 category: cleanup, EOF, truncation, timeout, closed, cancellation,
protocol, authority, emergency or other. Existing live-reader classification is
shared unchanged. Neither native error strings nor input contents are logged.

Inputworker, productlog, agent and residentagent tests pass. Tests distinguish
authenticated cleanup failure from wrong-channel frames, reject false clean
retirement and cover fixed categories including private-error redaction. This
does not establish the underlying cause of B's g393 source startup failure or
complete #50/#59. All ten open issues retain their remaining acceptance gates.

The controlled g394 disconnect/reconnect at 23:18:54Z yielded clean joined
source retirement on both PCs. Explicit Disconnect intentionally stopped B's
automatic resume, so A alone then waited for B's explicit Connect. This is not
a startup failure reproduction. B reconnection was requested from the user;
do not describe CONNECTING as healthy or the test as a completed reconnect.

No push, issue comment, branch/worktree, dependency, public API, DB, runner,
firewall or signing change. The private source wire format is unchanged.
