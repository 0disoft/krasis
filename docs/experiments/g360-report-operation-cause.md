# g360: Keep the selected operation and its original native cause together

## Reproduced defect

G359's new source-authority metadata is selected unconditionally ahead of a
more specific native cleanup operation. Joining hook-release/native 170 with
authority-console-token/native 5 then serializes the latter instead. Cleanup
still fences the service, but the diagnostic obscures why cleanup failed.

`TestConcreteCleanupReportCannotBeReplacedByAuthorityDetail` fails against the
actual g359 report_failure.go through a temporary Go overlay in .cache and
passes against the current correction. The retained device baseline is g357;
g359 was packaged, not applied. This is a reproduced local candidate defect,
not evidence of the earlier real source-hook failure's cause.

## Change and validation

Desktop/source failure helpers now return the selected deepest operation with
its own cause. Their existing operation-only functions keep the same behavior.
The report encoder extracts the numeric native error from that selected branch,
instead of the first native error anywhere in a joined error tree. Source
authority details refine generic source/worker containers, but cannot replace a
concrete hook/raw/cursor/desktop cleanup operation. The fixed report layout,
operation vocabulary, failure status and service recovery fence are unchanged.
No raw error text crosses the support boundary.

Desktop and source tests cover a preceding unrelated native error while
retaining the chosen operation's native 170. Report tests cover the same branch
pairing and encode/decode concrete cleanup failures alongside authority errors;
ErrRecovery and the service fence remain. Inputdesktop, wininput, inputworker,
productlog, logindiag and localapi tests pass, as does
`go vet -unsafeptr=false` for those packages. The old-code overlay deliberately
fails the new focused regression; the corrected-source run passes.

Bugfix, desktop-app and error-message-integrity review apply. G360 is reserved
locally, not built or applied. Both PCs retain g357 awaiting its attended movement
comparison. No public API, DB, runner, dependency, repository-hygiene, input
policy, deadline, input replay or permission change; no push, release, CI,
installation, process restart, reboot or live input injection is included.
The original long-term source stall and human-visible movement latency remain
unverified. The new helpers are internal package APIs for diagnostic selection;
they return causes to the existing numeric projection, not to UI/CLI messages.

## Packaged candidate

G360 at source revision `7ab3a4aa77c90cd5bc519ff334cea61b58142a07` now passes
the workspace build, including 85 frontend tests, native assets/UI compilation,
installer provenance, archive roundtrip and eight-file/icon verification.
Offline update verification returns valid/ok and archive size 27895442 bytes.
Independent PowerShell SHA-256 values:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 82c34662734260aacdf1c5de955e515b11a7d07c150cb164245dd5a47984388b |
| krasis-agent.exe | a84aed84c29a38070ecec820653ec7150dc0594cf4b6530e7c37f470cc9a4dbf |
| krasis-cli.exe | 489f45fc6a56c9265db36e92f5904d27bee5e123f79aeb49e357e12265da1edc |
| krasis-install.exe | 600e9a8feef10ae15caee916f063cec9f706cd315ab9002071af4b24ddb1b0d6 |

The earlier "not built" statement describes the source-validation checkpoint;
the candidate is now built but remains unapplied. G357 device state and its
pending attended movement comparison are unchanged. No additional live trial,
service restart, installation, push, release or CI run was performed. Build and
artifact checks do not establish a human-visible movement improvement or
resolution of the original long-term source-hook failure.

## Paired rollout

The g357 attended comparison remains unavailable; it is not inferred from the
earlier g356 reply. To make the failure-evidence changes available on actual
devices, the verified g360 candidate was applied through the existing product
update path. B transfer `5d2b23ff5ce287912c2af35330f87b41` reaches UPDATED for
generation 360 and exact revision `7ab3a4aa77c90cd5bc519ff334cea61b58142a07`.
A then opens the verified bundle through normal single-instance UI replacement.
The UI and agent transition succeeds without force-killing either process.
The saved B connection is started once after A reaches LOCAL.

A doctor now reports healthy/CONNECTED/PROTECTED/CONFIGURED, matching UI,
launch record and installed agent at the g360 revision, with the protected
service RUNNING. Remote update status observes B ONLINE at that same revision.
Paired collection `c431900cec45102b4811a534d20edc45` is RECEIVED with both
support files created at 2026-10-19 17:22:10-12Z. Both service/input-health
records are READY. Install receipts are A 17:21:57.3625265Z and
B 17:21:19.1067894Z, each result 0 / complete / recovery not_needed. Automatic
installer process exit/stdout/stderr were not captured independently. B doctor
was not run through a remote shell; its fresh service/input records and exact
agent version are the available remote evidence.

No g360 fault/failed/session-end event is retained in this short startup window;
control.previous.jsonl remains incomplete on both PCs. This does not establish
attended input behavior or long-term stability. A new g360 idle/continuous
movement/three-return question explicitly supersedes the pending g357 question;
its human result and corresponding movement diagnostics remain outstanding.
The hot input performance changes under comparison remain those of g357.
No reboot, manual service start, repeated installation, input replay, trust or
security change, force kill, push, public release or CI was performed. Ops-change
review applies to this rollout in addition to the source review procedures.

## Resumed movement review

The operator now reports that g360 is generally smooth. Keep that report as
the attended result; it is not a timed measurement or a lock/PIN regression
result. No new candidate or runtime change was made for this review.

Current local doctor returns healthy / CONNECTED / PROTECTED / CONFIGURED,
with the protected service RUNNING. Peer update status observes B ONLINE at
the same generation 360 and source revision. Fresh paired collection
`1a37109e4ed03cdd39ce5675e1647ff4` is RECEIVED; both support files were
created at `2026-10-19T19:07:29Z`, with input-health READY observations at
19:07:13.7679678Z (A) and 19:07:14.9182928Z (B). This is a remote support
observation, not a remotely executed B doctor. Rotated logs retain different
intervals, so use the earlier immutable collection below for the failures.

Collection `e2e043ea7fb2cfdfc82797cf0d4bc337` narrows the two g360 connection
terminations to distinct B admission failures:

| B observation (UTC, 2026-10-19) | Corresponding A observation | Finding |
| --- | --- | --- |
| 17:47:38.8575066 `installed_input_admission_blocked_active`; 17:47:38.8601246 triggering `fault_control_request_authorize_failed` | 17:47:40.6015334 triggering `fault_remote_eof` | `privilegedLeaseInput.open` found a non-nil client under its mutex and rejected the new lease. Why that client remained active is unproven. |
| 17:52:16.8447897 `installed_input_connect_failed`, `installed_input_stage_pipe_open`, `installed_input_native_2`; 17:52:17.0767191 triggering `fault_control_request_authorize_failed`, native 2 known | 17:52:18.5811811 triggering `fault_remote_eof` | The protected input named-pipe open failed; this is not evidence of a missing truststore file. Why the endpoint was unavailable is unproven. |

The authorizer wrapper opens the privileged channel after the peer policy
check. Consequently the broad request-authorize failure label covers channel
admission as well as policy authorization. In this evidence, the adjacent
fixed stage codes identify the narrower branches. Wall-clock timestamps from
different machines do not establish network or propagation latency. Neither
failure is classified as an intentional manual disconnect from these records;
some control-log history is incomplete.

Two latency interpretations also need to stay separate from observed defects:

- `handoff.ReentryGap` measures from the preceding attempt's retirement to
  the next attempt's creation (`Retire` to `Begin`), including time between
  crossings. The reported roughly 2.47 seconds is not an entry processing
  duration.
- `boundary_dropped_not_ready` is emitted when outbound/bidirectional state
  cannot admit another boundary request, including an already active source,
  a pending transition, or a held-input reverse lease. Its 65 records do not
  establish 65 missed physical crossings or 65 worker startup failures.

Next causal work should correlate the first failure's existing lease identity,
matching return/disarm and close completion, and the second failure's service
pipe retirement/rebind interval. Do not clear an active lease or retry unknown
input outcomes on the strength of these counters alone.

Validation here consists of doctor, peer update status, paired collection,
fixed-code log comparison and source-path inspection. `git diff --check`
validates this documentation-only change. Source tests, builds, new rollout,
service restart and attended lock/PIN testing are skipped because executable
behavior is unchanged. No API, DB, runner, dependency, product policy or
source-of-truth contract changed; no push, public release or CI run occurred.
