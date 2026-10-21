# g420: overlap independent native input/recovery startup

The user's g419 trial reports delay crossing into B. Paired collection
`bd7dd944e83e41bebe2034c868f9a447` has nine completed installed-receiver
profiles: ReadyWait is 234.1–436.1ms, including bootstrap-to-ready of
225.0–276.0ms even for the first eight prepared console children. These local
spans overlap and do not measure physical input-to-display latency.

The protected worker started its input child and waited for complete readiness
before launching its independent release-only child. Both authenticate the same
fixed protected images/owners independently; neither requires the other's
startup result. Only the completed pair can be published to the journal owner.

Start both fixed children concurrently, retaining the same inherited pins until
both launch calls join. Failure cancels the sibling's startup but still joins a
late native completion and retires every created owner. Missing retirement
confirmation keeps ErrRecovery. Caller cancellation cannot publish late ready
children. The input/release capabilities, per-command native authentication,
ordered injection, authoritative journal and retirement-before-key-up remain
unchanged. No native input is run in the new startup goroutines; readiness uses
the existing non-injecting heartbeat. Windows handle allowlists, job bounds,
bootstrap bytes and all wire/timeout policies are unchanged. Login startup is
outside this change.

The extracted serial production path fails the regression: independent 100ms
and 200ms startup takes 300ms instead of 200ms. It also cannot start/join a late
sibling when the first role fails. The concurrent implementation passes the
overlap, fail-either-role, confirmed/unconfirmed retirement and canceled-caller
cases. Controlled delay savings do not establish actual device savings; paired
application and timings follow below. No push, comment, remote ref, API/DB,
runner, dependency, firewall or signing configuration change.

The initial wider inputworker run failed only the three portable-pair recovery
fixtures at StartPortableProcesses; an isolated rerun reproduced the same
immediate ErrExitTimeout. A was running a real PORTABLE session at the time.
The production launch fence uses the fixed Local\\Krasis-Portable-Input-v1
object and returns that sentinel on ERROR_ALREADY_EXISTS, so these fixtures
cannot start beside a real portable owner. After the existing safe-stop
acknowledgement, the unchanged recovery fixtures and new pair tests pass.
The rest of the wider inputworker checks passed; crashguard and logindiag pass.
The failed initial run is not counted as a full suite pass. No test threshold,
ownership name, production fence or acceptance condition was weakened.

Pre-update collection `ea9f576dd879112ef1f4b4ce3a7edfa0` also shows g419's
earlier 10:02:04Z source heartbeat failure and missing worker report, followed
by a stopped protected service and PORTABLE fallback. This startup performance
change does not establish a repair for that separate source-worker failure.

## Paired rollout and reconnect trial

A's repair exited 0 with receipt 10:14:25.9868564Z; B update job
`dfb8b417ad58e1741d5b151106a4d15f` reached UPDATED, receipt
10:15:04.8059931Z. Both input health samples reached READY at revision
`cab8e796101002e1254b39f7ee512f97085339cd`. The user then reported a
connection followed by RECONNECTING.

Collection `0393008cc59ea926cc46631f46654527` shows A's source-hook failure
at 10:18:53Z and B's remote EOF/reconnect. A's native capture/sender/interrupt
joined and sent clean retirement, but its independent worker report failed
with source-command-loop (no native code). Later 10:23:43Z local and
10:23:40Z peer snapshots show four A source exits versus four clean B source
exits. A's later reports include source-reader-authority and one
source-reader-timeout/recovery. Generic authority/loop records do not identify
which native query expired; this is not proof of a network-first failure.

B receiver profiles have ReadyWait 174.407ms and 165.069ms, AuthorizeTotal
178.694ms and 169.150ms. Only the first ends ok; the second is incomplete.
This small sample suggests lower startup waits than g419, not an end-to-end
latency claim or a stability pass. No issue is closed from this trial.
