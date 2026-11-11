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
