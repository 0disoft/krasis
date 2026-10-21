# g417: capture local handoff stalls with a fast network exchange

## Actual g416 trial

The installed g416 A UI and agent were gracefully restarted using the verified
launch record and pinned runtime exit. No executable was replaced. A's agent
logged `runtime_trace_enabled` at 09:06:27.0123177Z on 2026-10-21, then connected
to B. The first restart helper stopped after the already-idle agent returned
`no_active_session`; accepting that terminal idle result allowed the same
verified restart to finish. No forced termination was used.

The user reported slight stutter without a major problem. Paired collection
`8885f0e5d856e81e6fdaec9c9404d56c` at 09:07:50Z contains 12 completed source
profiles on A (807 counted frames) and 12 receiver profiles on B (778 counted
frames), all ending `ok`. Receiver profiles identify the native backend; this
does not prove the installed protected backend's fault acceptance. Profile
counts reflect local/coalesced observations, not a packet-loss comparison.
There is no fault/failure in either exported event history after 09:06:49Z.

A's one-minute summary measures maximum queue residence 70.742ms and control
dispatch 225.175ms, with timer lateness 34.632ms. B measures maximum input
injection/dispatch about 9.056ms and timer lateness 6.871ms. These software
timings do not measure physical input-to-display latency or establish why the
user perceived stutter.

The third source profile has request-to-grant exchange 35.488ms but local
capture activation 223.420ms. Another profile has source deactivation 137.952ms.
Neither the >=80ms exchange trigger nor >=1s timer trigger requests a snapshot
for those cases. A's only trace remained the older October 19 snapshot; the live
g416 recorder also produced no save/expiry event during a bounded observation.

## Change and verification

After summary emission, request the existing opt-in snapshot when a measured
source capture activation or deactivation span reaches 80ms. Preserve the
network trigger. Each span qualifies independently; do not add nested spans,
infer measurements from nonzero values, or use intentional inter-input idle.
The input/cleanup paths acquire no new lock, perform no trace/disk I/O, and
receive no new diagnostic callback. The existing recorder remains opt-in,
single-use, 15-minute bounded and local-only; no raw trace enters peer export.

The regression uses the observed 35.488ms network / 223.420ms capture case and
the 137.952ms deactivation case. It also checks missing measurement bits,
receiver exclusion, the existing network threshold, and no sum of separately
subthreshold capture spans. Residentagent, handoff and runtimestall suites pass.
This fixes a diagnostic blind spot; it does not claim the stutter or trust
freshness/held-input failure is fixed. No issue is closed from this change.
No public API/DB, wire format, dependency range, runner, firewall, signing or
remote-ref change. Paired bundle/application evidence follows below.

## Bundle, application and observed recurrence

Local source commit is `8f333863a90f708c49263f508345c90ed166841c`. UI tests
(85), native frontend/Windows binaries, installer provenance, archive smoke and
eight-file package verification pass. Offline verification accepts g417,
28,295,106 bytes. Localapi/productcli suites also pass. No hosted CI was run
because no source was pushed; physical/signing matrices remain incomplete.

A's automatic installed update completed at 09:15:20.0257167Z, result 0,
recovery not_needed. Installed agent SHA-256 matches the g417 bundle. A's
g417 agent logs `runtime_trace_enabled` at 09:15:28.2713218Z. B update
`f88667cdd00220fe3c73bf2bc3431a98` reaches UPDATED; fresh peer version reports
g417/8f33386 ONLINE. This proves B's user agent/UI update only: collection
`5014cd08ebbe73f9fb5b7d7ea3428dfa` still shows B's older protected installation
receipt and SERVICE_RECOVERY_REQUIRED (1066) at 09:18:31.8788886Z. Its installed
protected service remains failed. Do not describe the full protected installation
on B as successfully upgraded or repaired.

A's first g417 run then fails at 09:16:42.3481484Z with source heartbeat
response failure, response reader ended and `fault_source_hook_failed`.
The source worker's receipt identifies `source-reader-authority`, with capture,
sender and interrupt joined but retirement delivery_failed/report failed.
This differs from the earlier B trust-freshness event and remains unresolved.
The failure trigger (already present before this change) saves a 2,527,432-byte
trace at 09:16:42.6069478Z. It does not demonstrate the new local-transition
trigger exercised in production; that trigger is covered by the regression.

The raw trace was copied to `.cache/analysis/g417-source-heartbeat/failure.trace`
before offline analysis; no raw trace was committed or transferred. Go 1.27
rejects the old `-d=1` syntax; `-d=parsed` succeeds. Parsed ranges contain ten
complete STW spans, none incomplete, with maximum 31.894108ms (then 14.140619ms).
This agent trace does not demonstrate a multi-second agent GC stop at this
failure, nor can it establish the separate source child's runtime behavior.
Syscall pprof sums delays across goroutines: 31.52 seconds total, including
bounded process waits and pipe reads. That sum is not a single blocking call's
duration. Scheduler profile total runnable delay is 457.92ms across goroutines.

The automatic recovery replaces A's faulted process at 09:16:56.9987955Z and
reconnects at 09:16:58.1656704Z. Later doctor reports matched g417 versions and
CONNECTED; status identifies PORTABLE fallback. Protected operation and sustained
reliability are not proven by that healthy connection projection.

Next investigate the installed source authority/presence failure using its
exact-owner receipt and, if needed, the source child's own bounded trace. Restore
B's failed protected service through the existing verified repair path when
local execution/consent is available. Preserve all authority and retirement
fences; do not extend timeouts or erase fault latches from this evidence alone.
No second fix was started after the ten-minute cutoff. No issue was closed,
no comment posted and no remote ref updated. The user reports only slight
stutter in the earlier g416 trial, not current g417 physical acceptance.
