# g418: retain the source presence watcher's authority failure

The g417 installed source worker reported `source-reader-authority` after the
09:16:42Z failure. Its independent presence watcher unconditionally replaced
every failed check with ErrAuthority. This discarded the already-sanitized
process/console/service operation and numeric Win32 result before the worker's
final report. Existing authority-detail tests did not traverse this watcher.

The watcher now uses the existing sourceAuthorityResult projection. Known,
closed operation codes and their own numeric native result survive; arbitrary
callback text/errors are still discarded. The return remains an authority
denial. The change grants no input, retries no failed query and changes no
freshness, read, retirement, cleanup or recovery budget. It uses existing wire
operation values and adds no diagnostic payload, schema or sensitive fields.

The regression exercises the watcher, joined source cleanup and final report
projection for process-exited, console-session and service-pipe failures. It
checks that numeric error 5 stays paired with its operation, unknown private
details are removed, and uncertain cleanup still reports unsafe retirement.
The previous watcher fails the regression; the focused source presence,
authority and reader tests pass after the fix.

The user reports B's explicit agent recovery completed. Peer verification is
still pending: a concurrent paired collection returned outcome_unknown while
A was FAULTED. Local diagnostics separately record trust freshness expiry at
09:27:21.9766116Z and recovery deadline exceeded at 09:27:25.9812165Z. A saved
trace copied as `.cache/analysis/g417-source-heartbeat/trust-expiry.trace` has
file time 09:27:01Z, before that expiry; it must not be described as a capture
of the later expiry. The earlier 09:16:42Z trace remains in `failure.trace`.

This repairs failure attribution, not the underlying recurring session faults.
No issue is closed from the diagnostic fix. No push, comment, remote-ref,
signing, firewall, API/DB or runner change. Further validation/application
results follow below.

## Packaging and paired application

Source commit `974d984a42ac35cc5270e9259d2ce54daf562e81` passed the full
inputworker, residentagent and logindiag suites before packaging. The workspace
builder passed 85 frontend tests, the native frontend build, Windows executable
and installer builds, installer provenance, archive smoke and eight-file bundle
verification. Offline CLI verification accepts generation 418 and 28,295,135
bytes. No hosted CI or signing verification was performed.

A's g417 agent had already stopped after a desktop-switch event at 09:31:29Z.
An ordinary g418 launch staged replacement but the old UI logged replacement
launch failure. The existing verified older-UI shutdown helper closed the old
window, then reported main-loop unavailable for a concurrent background recovery
process. A subsequent ordinary g418 launch and verified `app repair` completed.
No process was force-terminated and no Windows Security dialog was automated.

A's installation receipt is successful at 09:38:49.9074498Z. Fresh doctor reports
matching g418 CLI, UI, resident and installed revisions, RUNNING service,
PROTECTED agent location and CONFIGURED protected input. B's transfer
`90c0431bd9a23675bf16ef2ffae9af11` reaches UPDATED and fresh peer inspection
reports ONLINE g418 with this exact revision. The SEND command lost its reply;
status and exact-ID wait establish delivery/application without a second SEND.

Paired collection `d8c1b1ff8f3b7f3f49c2622c4ddebbe3` includes B's successful
installation receipt at 09:39:45.1443751Z and inputHealth READY at
09:39:51.4368351Z. This verifies the user's reported recovery and the subsequent
protected installation, unlike g417's user-agent-only rollout. Later collection
`959c379c871b665110cf77af20c68100` also reports both inputHealth records READY.

## Recurrence and next work

After explicit reconnect, status briefly reports CONNECTED / PROTECTED. At
09:41:08.4115506Z A then logs installed_source_startup_failed, stage worker_ready,
native_unknown and startup_authority. The triggering session receipt identifies
source-hook / fault_source_hook_failed, without a known native error. At
09:41:12.4973215Z guard close exceeds its exit deadline and the session fails.
This is a startup-authority path, not proof that the newly fixed presence-watcher
projection ran. The recurring fault remains unresolved; READY installation
health does not establish a healthy session.

The g418 trace was saved at 09:41:03.5378525Z, before the startup failure. Its
2,073,721 bytes and agent log are preserved locally under
`.cache/analysis/g418-source-startup/`. Do not describe this single-use snapshot
as a direct capture of the later failure. Post-failure paired collection is
`740537abbcb27751b3c491301d9d9c3e`. Raw traces/support files are not committed.

No new implementation was started after the ten-minute cutoff. Next priority
is the protected source startup's worker_ready authority denial and subsequent
guard exit timeout, preserving authority and retirement fences. Review that
path before extending diagnostics again or changing any timeout. The exact
native cause remains unknown.

GitHub still lists #1/#4/#5/#6/#7/#15/#16/#20/#50/#59/#70 open. This diagnostic
fix and successful installation do not satisfy their remaining physical,
signing, endpoint-change or hosted-CI acceptance. No issue was closed, comment
posted, source pushed or remote ref changed. No API/DB, runner, firewall,
certificate-trust or repository-hygiene change. `git diff --check` validates
this application record; unchanged implementation tests are not repeated.
