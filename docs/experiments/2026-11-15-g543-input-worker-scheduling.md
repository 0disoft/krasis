# g543: cover authenticated input-worker scheduling dependencies

## Evidence that changed the target

After A's opt-in flight recorder was stopped, the user reported severe lag.
Collection 398cb652f5599a4c0e434680a6c09ce5 retains the same shared g542
connection and five new complete matched exchanges. A's request-to-grant is
2103.088, 1502.666, 847.878, 1377.583 and 1094.845ms. Corresponding B readiness
wait is 1354.679, 688.870, 764.125, 1194.833 and 759.368ms. A capture activation
is 13.496..30.722ms. These are local, nested measurements, not cross-PC wall
clock subtraction.

For the worst 2103.088ms entry, B authorization/connect setup is 1923.893ms.
The overlapping B diagnostic minute records control dispatch up to 2051.220ms,
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

## Paired installation and physical result

Built c3ab4f7e7192c7ef89b80148c291b6efbdc40df3 as g543. Packaging passed all
111 frontend tests, native UI build, input installer build, productpanel wire
transfer test, icon/single-copy verification and setup verification. Exactly one
B send, ID 0e2d03be818997242b014e33a2f77e35, completed UPDATED with all 29230198
bytes and the exact revision. A setup exited 0 / installed, protected installation
stage complete, recovery not_needed. CLI app-connect receipt
424e199120dfeb95547c7467b7a90f4d confirmed connected. Installation collection
a8bc18cc6d67b166d3a792f506f541b4 verified both services/input READY, matching
builds and a shared connection, with tracing disabled in both active runs.

The user reports that lag appears gone. Collection
747628d80d2db9591bcabd6a7d40b5b6 contains 12 uniquely matched complete/OK
source/receiver profiles and no incomplete pair in the current connection.

| Process-local span | g542 severe trial median / max ms (5) | g543 median / max ms (12) |
| --- | ---: | ---: |
| A request to grant | 1377.583 / 2103.088 | 41.211 / 98.619 |
| B authorization/connect total | 1230.495 / 1923.893 | 25.774 / 36.396 |
| B worker readiness | 764.125 / 1354.679 | 22.283 / 32.520 |
| B first input dispatch | 73.694 / 331.702 | 1.798 / 4.000 |
| B worst input dispatch per entry | 280.557 / 526.585 | 3.503 / 5.813 |
| Inner native handler | 146.460 / 253.475 | 1.643 / 4.449 |
| SendInput | 62.924 / 143.839 | 1.151 / 3.503 |
| Service reply-ready to app read-return | 36.257 / 323.964 | 1.070 / 2.358 |
| A capture activation | 20.760 / 30.722 | 16.115 / 41.616 |

This compares the five severe untraced entries after 13:46Z with the g543
trial, not every earlier g542 entry. Earlier mild g542 entries were already
40..112ms request-to-grant. The new result returns to that range, rather than
proving a permanent 20x improvement under all CPU workloads. Sample counts,
polling, system workload and observation duration differ; no causal effect size
is established by this uncontrolled before/after comparison. A capture maximum
has not improved against the selected severe-trial baseline.

Both sides preserve the same connection for about 122s after installation.
There are no new postconnection faults, reconnects, poisonings or repair-required
events. Both services and input remain READY and service start timestamps match
the immediate installation collection. A B startup reconnect before the final
shared connection is retained in logs and excluded from trial recurrence, not
hidden as a successful trial event. Two minutes and twelve capped profiles do
not establish long-duration stability or zero remaining stutter. Exact hashes,
selection and local software intervals are in g543-paired-crossing-review.json.

No further source change is justified by this short successful trial. Leave the
flight recorder disabled. No push/public release/remote CI was performed, as not
requested. Linux UI and a controlled CPU-contention experiment were not run for
this Windows-only change; native policy and physical product use were checked.
