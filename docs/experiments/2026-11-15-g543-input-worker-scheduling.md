# g543: cover authenticated input-worker scheduling dependencies

## Evidence that changed the target

After A's opt-in flight recorder was stopped, the user reported severe lag.
Collection 398cb652f5599a4c0e434680a6c09ce5 retains the same shared g542
connection and five new complete matched exchanges. A's request-to-grant is
2103.088, 1502.666, 847.878, 1377.583 and 1094.845ms. Corresponding B readiness
wait is 1354.679, 688.870, 764.125, 1194.833 and 759.368ms. A capture activation
is 13.496..30.722ms. These are local, nested measurements, not cross-PC wall
clock subtraction.

For the worst 2103.088ms entry, B's same-process control dispatch is 2051.220ms,
while A's request-written-to-grant-read is 2102.533ms and read-to-dispatch is
0.555ms. Sampled TCP smoothed RTT is 24.792/24.289ms with no observed retransmit
or timeout delta. Network and A read-resumption uncertainty remain, but they
cannot explain away B's independently measured two-second handling interval.
Thus the newest severe trial is dominated by B preparation, not A foreground
activation or the earlier traced-run GC pause.

The worst B input dispatch in this trial is 526.585ms: service-native handling
182.862ms, service reply-ready to app read-return 323.964ms. Its inner native
handler is 146.460ms and SendInput 62.924ms; these are overlapping measurements.
They do not prove 526ms of SendInput or mutex contention. System CPU samples in
B's retained trail are often saturated; runnable/OS/GC observations alone still
cannot identify a particular competing process or separate all native waits.

Both services and protected input remain READY and no new session fault,
reconnect or 1066 repair appears in this untraced connection. That does not
make the severe lag acceptable, nor prove long-duration stability. The earlier
g542 trace-specific 7.096s stop is recorded separately and tracing remains off.

## One bounded mitigation

g541 already covered the isolated capture child's Go scheduler and query-start
dependencies with NORMAL-to-ABOVE_NORMAL process scheduling. B's installed input
parent and native input/release children still ran at NORMAL. Individual native
thread promotion does not cover parent IPC, proof dispatch or Go continuation.

Reuse that exact best-effort scope for these three installed input processes.
Enter the input-parent scope only after full bootstrap/service/agent/lease
checks and after the capture/presentation return branches. Each native child
independently validates installation, service, agent, parent and lease before
entering. Prepared-only/unbound candidates and presentation remain unchanged.
The scopes stay through serving and joined proof cleanup and restore NORMAL
once. Existing explicit priorities or later observed external changes are kept;
denial and failed restoration never grant authority or trigger service repair.

There are at most two priority reads and two writes for each of the three
process lifetimes: at most twelve extra OS calls per input lease, zero per
input frame or proof query. No capacity, lease, pressed-state, authentication,
deadline, socket, GC or cleanup policy changes. This is neither REALTIME nor
HIGH priority. Other runnable work within those isolated processes shares the
class; GUI fairness must be checked in the next physical trial. Windows offers
no priority-class CAS, so a concurrent same-value external policy write cannot
be detected perfectly. Performance benefit remains unproven until paired use.

## Validation and delivery plan

Reuse deterministic tests for explicit policy, denied/failed promotion and
restoration, external changes and concurrent cleanup. Extend the owned native
Windows child test to verify the new shared scheduling entrypoint as well as
capture, without altering test-runner priority. Run relevant inputworker,
wininput, handoff, residentagent and agent tests plus scoped vet. Package one
committed g543 revision, send B once with its exact receipt, install A without
the trace environment and restore CLI connectivity. Keep full authentication
and native-owner joining in all failures. Remote CI/push are not requested;
Linux UI tests are outside this Windows scheduling scope.

Source of truth: docs/architecture/02-runtime-flow.md. No public API, DB,
dependency, repository hygiene or runner changes.

Prepackage verification passed the five listed Go packages and inputworker /
wininput vet with -unsafeptr=false, using the repository's verified runtime
overlay/toolchain. The owned native priority child covers both scheduling
entrypoints and restoration. Diff checks pass. The scope adds no new shared
mutable state; the existing concurrent-once cleanup policy tests remain in the
package suite. A new race-toolchain build was not needed for unchanged cleanup
synchronization. These tests confirm scheduling policy, not a latency gain.
