# Ubuntu CI package acquisition (2026-11-12)

## Failure and correction

Run [43058913597](https://github.com/0disoft/krasis/actions/runs/43058913597)
failed twice before Linux tests started. APT's runner-provided `mirror+file`
source reported `Temporary failure resolving 'azure.archive.ubuntu.com'`.
The update exited with warnings and retained old indexes; installation then
failed fetching the missing packages from that same source path.

`scripts/install-linux-ci-x11-deps.sh` uses canonical Ubuntu HTTPS archives
for this installation only, with the runner's codename and Ubuntu-managed
archive keyring. Main and universe are included (Xvfb is in universe).
Both update and install use the same private source file and fresh index
directory; default source fragments and binary metadata caches are excluded.
Update uses `--error-on=any`, so a transient acquisition failure cannot fall
through to an installation using stale indexes. Only acquisition reads get one
APT retry; HTTPS timeout is 20 seconds. The existing CI job limit is unchanged.
Temporary metadata is removed on success, failure or handled interruption.

## Validation

- `actionlint .github/workflows/ci.yml`, Bash syntax and `git diff --check`.
- Ubuntu 24.04.5 amd64 userland in the existing Debian TCG VM, bootstrapped
  from the official Ubuntu Base archive, verified against its published SHA256:
  `e77b6f10c2590cef872b33ee9f635a0e3fd1f57fb074c0e52b5c7f56147a0c86`.
- An injected update failure returns its original exit 42, runs no install,
  and leaves no temporary index directory. Non-Ubuntu exits 2 before APT.
- Real dependency installation passed with an intentionally broken default
  source present: all 74 dependencies installed and `Xvfb`, `xauth`, `xdpyinfo`
  were found. Original source-file hashes were unchanged and temporary indexes
  were removed. The cold Ubuntu Base installation took 421 seconds on TCG;
  this is not the pre-provisioned CI runner's installation benchmark.
  This is userland/package validation, not a
  new full Ubuntu VM or another desktop lifecycle/power test.

The first disposable-root attempt exhausted the VM's small `/tmp` tmpfs and
was not counted as a pass; its replacement uses the regular VM disk. CI uses
`RUNNER_TEMP` for index storage and excludes translation indexes.

External API, DB, runtime code, Windows CI and CI job count are unchanged.
Reverting the workflow call restores the previous runner-default APT behavior.
Complete failure of the canonical archives/DNS remains a visible installation
failure; signature or HTTPS validation is never disabled.

Reference: Ubuntu's [APT source and signature policy](https://manpages.ubuntu.com/manpages/noble/man5/sources.list.5.html),
[APT directory overrides](https://manpages.ubuntu.com/manpages/noble/man5/apt.conf.5.html),
and [update error handling](https://manpages.ubuntu.com/manpages/noble/man8/apt-get.8.html).

## Follow-up: PAM fixture startup notification

Run [43068044839](https://github.com/0disoft/krasis/actions/runs/43068044839)
passed package acquisition in 13 seconds, all Linux packages/vet and the
ordinary X11 input/clipboard tests. It then failed starting the first persistent
PAM login fixture after the eight-second startup budget. The quiet/collected
unit left insufficient diagnostics to classify its underlying PAM failure.

A cold-UID TCG probe took eight seconds, versus two seconds after manager
preparation, but this was not the Ubuntu root cause. The added journal in run
[43070518773](https://github.com/0disoft/krasis/actions/runs/43070518773) showed
the manager ready in 68 ms and a real PAM session already created before the
eight-second Type=exec startup timeout. Pre-starting the manager did not fix it
and that unnecessary bootstrap was removed.

Upstream [v255 setup_pam](https://github.com/systemd/systemd/blob/v255/src/core/exec-invoke.c#L1205)
leaves the startup exec fd in the PAM helper, whereas
[v257 explicitly closes it](https://github.com/systemd/systemd/blob/v257/src/core/exec-invoke.c#L1189)
to let the manager observe exec completion. The fixture avoids relying on that
EOF: Type=notify with NotifyAccess=main runs systemd-notify as the actual PAM
main process, sends READY and execs sleep without changing PID. Its eight-second
limit, real PAM/logind and cleanup remain; no linger or synthetic state is used.
Relevant journals are retained on startup failure.

The TCG probe passed in four seconds, verifying an active real session, correct
UID and an actual sleep main executable. A process without READY was rejected.
Bash syntax, workflow actionlint and whitespace checks passed; full remote
lifecycle execution remains the acceptance check. The notify/exec operation is
[documented for Ubuntu's systemd](https://manpages.ubuntu.com/manpages/noble/man1/systemd-notify.1.html).

Run [43072398757](https://github.com/0disoft/krasis/actions/runs/43072398757)
passed acquisition in 10 seconds and started both notified PAM fixtures. It
exposed the same inherited-exec-fd dependency in the Go integration suite's
actual agent launcher: product logs reached `agent_ready` in 9-39 ms while
the Type=exec unit remained activating until the eight-second test failure.
The launcher now uses main-PID notification too, with an explicit eight-second
unit startup timeout. Notification proves PAM admission, and the unchanged
separate CLI status probe proves the agent actually executed and is ready.
An explicit `--` keeps the agent's `env -u` options out of systemd-notify's
option parser. Process ownership, real sessions, scenario budgets and cleanup
are preserved. This only changes test infrastructure, not installed agents.

The current-source TCG receiver termination/recovery regression passed in
43.12 seconds: both UIDs entered real PAM sessions, held input and grabs were
released, the unsafe API stopped, and the same identities reconnected from a
new session. The initial broader TCG trial failed its second cursor-entry budget after
PAM admission had passed. The full-suite follow-up below supersedes that result.
An earlier trial accidentally used stale prepared product binaries and was
aborted/excluded. Linux cross-compilation, actionlint and whitespace checks
passed. Remote CI remains the acceptance check for the entire Linux suite.

## Follow-up: full TCG cursor re-entry and fixture resource contention

The entry probe queried diagnostics by launching a Go CLI on every 10 ms
observation, including before checking the native input echo. Under TCG this
competes with both real agents and Xvfb on the same CPUs. The successful path now
checks actual native cursor movement first; only declined-intent diagnosis polls
the real authenticated CLI, at most once per 250 ms. The deterministic scheduling
regression covers 800 observations in eight seconds: 32 scheduled counter reads
instead of 800, plus the separate initial baseline. It also tests immediately
before/at the interval and a backwards clock observation. No read is retried as a
mutation, and the existing declined-intent-only edge-warp rule is preserved.

Large build/test/product copies use a private directory on `/var/tmp`, instead
of consuming the guest's small RAM-backed `/tmp`. A direct VM probe confirmed
that sudo strips TMPDIR even with preserve-env; the actual test runner therefore
sets it using env after sudo. Only public executables are traversable by both
UIDs; identities and X11 cookies keep their existing private ownership/modes.
The disposable peer data directories remain small private `/tmp` directories.

Current-source Linux/amd64 products and the test binary passed on the existing
Debian 13 TCG VM (2 vCPUs, 2 GiB), using two real UIDs, PAM/logind, and independent
1280x720 and 1024x768 Xvfb displays:

- Full actual-agent CLI interoperability: 14.19 seconds, including cursor entry,
  relative motion, keyboard, buttons, all four wheel directions, return,
  re-entry, and held-input cleanup. Entry/re-entry took 1.109/0.637 seconds.
- All six actual lifecycle cases: 139.10 seconds total. Lock-session, lock-hint,
  and terminate-session each release input/grabs, retire the unsafe API, and
  reconnect the same identities in a new PAM session, on both source and receiver.
- Lifecycle cursor entries: 1.019-1.271 seconds. No scenario or cursor timeout
  was increased. This is emulated test execution, not a real-laptop latency result.
- The existing 1-vCPU/1-GiB TCG guest also passed full actual-agent CLI
  interoperability in 29.59 seconds, including re-entry and normal disconnect,
  using the corrected post-sudo disk temporary directory.
- Interval boundary regression, Bash syntax, workflow actionlint and whitespace
  checks and Linux residentagent vet passed. An isolated sudo/env temporary-file probe used the disk fixture
  and removed its file/directory.

The experimental persistent CLI poller was removed: it did not establish a fix.
SIGQUIT stack captures and failed exploratory trials remain ignored under
`.cache/linux-validation/desktop/`; they are not acceptance passes or source
artifacts. The captured stacks did not establish an Operations mutex deadlock.

Runtime API/DB contracts, installed runtime code, Windows CI, CI job count,
eight-second cursor checks, 110-second scenario budgets, and the eight-minute
Go suite timeout are unchanged. Revert the two test-infrastructure files to undo
this change. Remote CI on the resulting commit is the final acceptance check.

### Remote acceptance: initial TLS round registration race

Run [43090802221](https://github.com/0disoft/krasis/actions/runs/43090802221)
passed Windows but failed the ordinary Linux package suite before X11 began.
`TestProductCLIPairsNewDevicesAndPreparedTransportConnects/both_sides_renewal`
reported client EOF, a ten-second server wait, and all pre-auth counters zero.
`ListenSource.acceptLoop` intentionally closes connections arriving before
`Next` registers an admission round, without incrementing admission counters.
Starting Next in a goroutine does not establish that readiness barrier.

The test now retries only an initial TLS EOF while every pre-auth counter is
still zero, using the unchanged parent deadline. Any admission, rejection,
authentication failure, other error, or cancellation stops that retry. The
listener/dialer, actual certificate/trust validation and both endpoint completion
checks remain. No production retry, write, input or authentication policy changes.
Deterministic cases cover the pre-round EOF, all eight nonzero counters, original
error identity and cancellation before/during retry. The real three pairing and
renewal scenarios plus these cases passed on Windows and the two-vCPU TCG guest.
