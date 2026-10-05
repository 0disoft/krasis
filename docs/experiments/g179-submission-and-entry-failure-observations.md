# g179 native submission and entry failure observations

Revision: `08c018099f9ecbc98ba19fd33a8080d5eeecf983`.
These operator exports are an uncontrolled observational cohort.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 16:13:07Z | `A897C18098EEE1A4ED31CF66AD9B55716DEBF883F0BE58EDF22C79A149A80ABA` |
| B | 2026-10-05 16:13:02Z | `144B7C3CE78BC4BC209534BA1243B22B03BB22DC9CDF4C6978796F81B36936DF` |

## Successful retained profiles precede a real reconnect

Both hosts retain twelve complete handoff profiles ending `ok`, all before the
later connection interruption. Profiles end at 16:12:10.7835333Z on A and
16:12:10.7067901Z on B. Local attempt ordinals do not independently pair operations
across hosts. The twelve-attempt limit means the later failed entry has no retained
handoff profile; do not describe the whole trial as failure-free.

Event identities are unique within each host. Frame/histogram conservation passes:
A 1,254 [1244,9,1,0,0]; B 1,248 [50,1184,14,0,0]. Each export has one incomplete
historical log, with no current-revision events in that log. All twelve retained
B startup receipts report preparation decision 1 (`used`).

B's same-host event sequence includes:

| UTC | Code and observed meaning |
| --- | --- |
| 16:12:38.2390351 | `installed_input_lease_ready` |
| 16:12:38.2453261 | `injection_failure`: operation `entry`, certainty `OUTCOME_UNKNOWN`, cursor stage `mismatch`, counts unknown |
| 16:12:38.2590223 | `reconnect_control_dispatch_failed` and `RECONNECTING` |
| 16:12:40.0188857 | `reconnect_acquire_tls_handshake_failed` |
| 16:12:40.4703313 | `CONNECTED` |

A separately records `reconnect_remote_eof` and `RECONNECTING` at
16:12:38.3032496Z, followed by `CONNECTED` at 16:12:40.5157691Z. These compatible
timelines are not cross-host latency measurements. Earlier B network-acquisition
timeouts and a manual disconnect precede its first connection in this revision.

After the final connection, neither host records another failure through export:
approximately 26.5 seconds on A and 21.5 on B, calculated only within each host.
There are no new retained profiles after that reconnect. This short tail establishes
observed reconnection, not successful subsequent crossings or permanent stability.

## Native submission occupies the retained slow apply

B has 24 retained worker detail vectors (first and worst), all with one measured
native submission and no saturation. Presence, bounded ranges, containing-worker
bounds and nested submission/apply bounds pass. A's source vectors have no worker
detail, as expected. First and worst can select the same input and are not a
distribution of all inputs; worst selects dispatch duration rather than syscall
duration.

| B selection and part | Measured vectors | Min (ms) | Median (ms) | Max (ms) |
| --- | ---: | ---: | ---: | ---: |
| First submission | 12 | 0 | 0.602 | 2.382 |
| First apply | 12 | 0 | 0.602 | 2.382 |
| First snapshot | 12 | 0 | 0 | 0.635 |
| Worst submission | 12 | 0 | 1.1935 | 17.709 |
| Worst apply | 12 | 0 | 1.1935 | 17.709 |
| Worst snapshot | 9 | 0 | 0 | 0.580 |

Measured zero is below microsecond resolution, not missing work. Absent presence
bits are excluded. Equality of apply and submission at reported resolution does
not establish zero overhead outside the call.

B's slowest retained worst vector, local attempt 4, ends `ok`:

- Dispatch 24.070ms; ClientTotal and ClientResponse 19.005ms each.
- ClientAuthority is measured zero microseconds.
- Worker authority/scope/apply are measured 0/0/17ms (floored milliseconds).
- Apply 17.709ms and native `SendInput` submission 17.709ms, unsaturated.
- Subsequent cursor snapshot is absent on this operation.

The matching receipt locates the long worker apply inside the native submission
call. The Windows-internal reason (hooks, scheduling or other native work) remains
unobserved. This is not an end-to-end pointer presentation measurement, and the
g178 and g179 workloads do not establish a performance improvement or regression.

Another B worst vector, attempt 11, has ClientAuthority 9.902ms, reported pipe-owner
subspan 9.902ms, apply/submission 1.262ms and Dispatch 11.165ms. Authority remains
a separate delay candidate. Do not combine maxima from different inputs or subtract
nested/process-local spans to invent a scheduler or network residual.

## Entry failure source boundary and next priority

`internal/wininput/input_adapter_windows.go:PrepareEntry` reads the prior cursor,
computes the entry target, writes it once, then reads it back. Stage `mismatch`
means the readback differed from the target after a successful cursor write/read;
it does not mean `SendInput` failed. This entry uses cursor positioning, a separate
path from the retained native submission samples.

The source routes this error through cursor rollback before retiring the protected
lease (`internal/peer/sender.go`). The export contains no rollback failure, but
does not retain a positive restored/not-restored outcome for this entry, so recovery
must not be asserted from the absence of another error. Whether a PIN dialog,
cursor constraint, physical cursor movement, desktop switch or geometry change
was present is not established by the privacy-safe logs.

Prioritize this observed entry failure over speculative `SendInput` optimization.
Investigate the set/read desktop and cursor-constraint boundary, retaining rollback
ownership and outcome certainty. Do not ignore mismatched readback, replay input,
reset clipping or weaken protected input authorization to suppress the reconnect.
PIN-context confirmation and focused native evidence are still needed to select
the actual correction.

Validation: exact-revision isolation; host-local identity and timeline review;
frame/histogram, grant duration and byte-gap conservation; startup selection
invariants; worker detail and nested native timing presence/saturation/bounds;
source inspection; `git diff --check`. Skills: desktop-app and
performance-measurement-integrity-review.

Runtime tests, rebuilding and new native PIN/rollback trials are skipped for this
evidence-only document. No product version, API, database, protocol, installer,
runner or security-policy changes. Intermittent cursor-entry failure and native
submission delay remain unresolved.
