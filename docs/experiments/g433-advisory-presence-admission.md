# g433: keep advisory presence outside heartbeat serialization

g432's source exit history on A now distinguishes failures previously collapsed
to a late query-closed receipt. At 2026-10-22T12:18:42.5571099Z the heartbeat
ends at reply_gate, maximum bucket ge1s, with source-command-heartbeat and joined
capture/sender/interrupt. At 12:18:03Z and 12:20:38Z separate failures end at
agent_presence, maximum bucket ge1s. Paired evidence collection:
6eee8d70566cb4d2fec85e2de71ddf39. Do not conflate these stages or claim one fix
explains every failure.

The advisory boundary sender still performed an optional presence query while
holding the shared writer gate. Its one-second query budget could consume a
heartbeat reply's one-second gate/write budget. Existing tests covered the
full authority preflight and a heartbeat after optional-query expiry, but not
a concurrent heartbeat during the optional presence query.

A deterministic regression blocks that advisory presence query and requests a
heartbeat concurrently. Before the change, the heartbeat cannot arrive within
50ms and the boundary takes the first wire sequence. This reproduces a real
serialization defect; the live report alone does not identify its gate holder.

Move optional presence into advisory preflight, before the full authority check
and immediate TryLock. A contended gate discards both results and retries fresh
checks after waiting without authority. The final full proof includes native
process/console/logon validation. No native presence query remains inside the
advisory wire gate. A slow unsent optional query remains owned and bounded;
actual rejection and attempted/partial write failures retain their outcomes.
No input, reply or retirement path bypasses its existing authorization.

The same regression now receives the heartbeat first while the advisory query
is still blocked. Another regression revokes full authority during that query
and verifies that no advisory packet or sequence escapes. Deadlines, process
query capacity, input ordering, wire formats and retirement proofs are unchanged.

Validation: inputworker PASS (22.297s), logindiag PASS, residentagent PASS
(8.308s); focused added regressions, packaging and application are recorded in
the paired rollout result. The portable pair fixed-owner fixture is skipped
while the real product owns that resource. No hosted CI or physical input
acceptance is inferred from automated results.

Next priority remains agent_presence ge1s: distinguish queue admission from
native/kernel work without widening deadlines or caching successful authority.
Trust freshness, initial source startup and B entry readiness remain separate
observations. No public API, DB, dependency, runner, repository-hygiene or
source-of-truth specification changes. All commits remain local; no push.
