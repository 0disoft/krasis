# g541: cover capture scheduler dependencies before query dispatch

## Evidence and diagnosis

g540 collection `6f4299ce90ae2d397ca8c3f2f92c3bb1` retains ten complete
crossings. A activation median/max is 51.917/135.798ms; foreground exchange
median/max is 33.817/57.962ms. The worst activation includes 116.075ms of
worker activation, 17.550ms post-to-owner handling and 57.962ms foreground
exchange; request/reply pipe delivery is only 0.117/0.068ms. These are nested
durations, not additive independent delays.

At 11:34:56Z a source presence query expired in `scheduled`, before its callback
entered `checking`. That excludes attributing the whole timeout to SCM. The
source reader retired with all native owners joined and `clean_sent`; the
joined refresh reached protected capture READY 1.786s later. This was not a
transport reconnect, service 1066 failure or administrator repair. Earlier CPU
snapshots do not overlap the incident and do not establish CPU causation.

`startAuthorityQuery` previously started a goroutine and only inside that
goroutine locked/promoted its thread. Native capture, monitor and IPC owners
were individually promoted, but their Go scheduler and pre-callback dispatch
dependencies remained at the normal process base. A query could expire without
ever entering the individually promoted region.

## One bounded scheduling change

After authenticated bootstrap, fresh service/lease proof and agent validation,
only `lease.Capture` enters a NORMAL to ABOVE_NORMAL process-priority scope.
The isolated capture child covers its scheduler dependencies before queries
are launched. GUI, network agent, service and input/presentation worker roles
do not enter this scope. No deadline, proof, capacity, lease, suppression,
protocol or fail-closed retirement policy changes.

Microsoft documents that process class and relative thread priority jointly
determine base priority: ordinary threads move from 8 to 10 and existing
Above Normal owner threads from 9 to 11. This is a bounded scheduling hint,
not realtime priority or a latency guarantee. See
[Scheduling Priorities](https://learn.microsoft.com/en-us/windows/win32/procthread/scheduling-priorities).

Promotion only applies to an observed NORMAL class. Existing explicit policy,
unknown query results and denied promotion are preserved. Deferred cleanup
restores once, including concurrent repeated cleanup; an observed later policy
change is preserved. Restoration failure cannot request service repair and
lasts only until the one-lease child exits. Windows has no priority-class CAS,
so an external concurrent/same-value policy write cannot be perfectly identified.
There are at most two priority reads and two writes per child lifetime, with
zero extra per-input/per-query calls. Runnable capture background work also
shares the higher base; paired trials must check fairness and regression.

## Verification before packaging

- Tests passed for inputworker, wininput, handoff, residentagent and agent.
- Eleven deterministic priority cases passed, covering non-normal classes,
  query/set/restore failures, external changes and eight concurrent cleanups.
- A real owned Windows test child verified promotion and restoration, keeping
  the runner's process priority unchanged.
- Scoped inputworker authority/source-query/priority race tests passed with
  CGO and the existing portable LLVM-MinGW compiler.
- Vet passed for the five packages using `-unsafeptr=false` repository policy.
- Real Windows foreground smoke passed all five activations with owner checks;
  activation ranged 24.5271 to 58.9582ms. This fixture does not enter the new
  authenticated bootstrap scope and does not measure its performance effect.
  Native rejected attempts remained visible and fallback retained validation.

Source of truth updated: `docs/architecture/02-runtime-flow.md`. No API, DB,
dependency, repository hygiene or runner changes. Linux and remote CI are
skipped for this Windows-only scope. Controlled CPU-load and new physical
crossing tests remain necessary before claiming a latency reduction.

## Delivery

Package this committed revision as g541 without overwriting older artifacts.
Send B once, retain its exact update receipt, install A, then verify matched
CLI/agent/UI/launch/protected installation, READY and CLI connection on both.
Rollback uses a new generation from the previous source through this same
installer process. Push and public release are outside the current request.
