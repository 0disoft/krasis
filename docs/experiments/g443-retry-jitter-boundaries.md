# g443: deterministic retry jitter and safe duration saturation

Supervisor time, waits and connection/session I/O were already injectable, but
retry jitter still called the global math/rand source. Keep Delay(attempt) and
the comparable BackoffPolicy unchanged; delegate to an internal helper and add
an optional SupervisorConfig.Random source. Missing Random retains math/rand;
cryptographic identities and certificates never use this source. Reject injected
samples outside [0,1), including NaN/infinities.

A deterministic regression reproduces a duration boundary bug: Initial and
Maximum at MaxInt64-1 with midpoint jitter convert the rounded float to a
negative duration, which the lower clamp turns into 1ns. Cap the floating value
before integer conversion. The default 250ms/30s policy was not affected.

Tests cover zero/midpoint/just-below-one samples, invalid samples, one/two
nanosecond and MaxInt64 limits, attempt 0/1/MaxUint32, no random consumption
without jitter, and injected randomness through the actual supervisor's failed
acquisition/retry path. The saturation regression failed before the cap moved.
Related agent/loginagent/residentagent tests validate the optional dependency.
No new required argument, wire protocol, security policy, DB, dependency, runner
or repository hygiene change. Local main; no push. Packaged together with the
independent app-open observation change in g443.

Agent 1.976s, loginagent 2.433s and residentagent 6.317s passed; the production
supervisor test confirms the injected midpoint source is consumed once and
produces the exact initial retry interval. Paired rollout and skipped checks
are recorded in g443-app-open-observation.md and its result file.
