# ADR 0066: Product leases use the installed input service

## g350: Distinguish request handling from the preceding dispatch

The same generation freezes heartbeat progress at failure notification. Optional
`runtime_gc_trail.heartbeat` contains one sender and receiver attempt: sender
coordinator queue/acquire, TLS write start and terminal; receiver complete frame
read, coordinator queue/acquire and terminal. A terminal write failure is marked
before cleanup, never as sent. The last successful local completion survives a
later queued attempt. The original policy-clock sent/received observations remain
separate and cannot certify write completion or renewed lease authority.

Callbacks publish fixed atomic fields only. Each serial producer has a sequence
fence; failure snapshots retry at most three times per leg and omit a busy leg
instead of blocking or combining attempts. Notification captures by value before
cancellation can rewrite progress. The sampler logs it through the existing
eight-trail budget, with no per-heartbeat disk output, new peer frame, identity,
input content or endpoint. New connections clear heartbeat evidence after the
previous session workers join; old-peer send/receive ages no longer appear as
observations on a new connection. Policy timeouts and cleanup are unchanged.

Focused tests distinguish an 80ms coordinator wait from a 40ms write, preserve
cancel/expired/malformed and write-failure outcomes, freeze pending work before
cleanup, reject mixed/invalid evidence and verify allocation-free callbacks.
Peer, agent, productlog and residentagent tests and vet pass. These are local
protocol/telemetry checks, not evidence that the g349 timeout cause is fixed.

Paired g348 evidence `e5f5327ed3f74e9a2bea2ebfb55cc01a` separates two
latency paths. Exchange `12888f8b41a76f196979cd7ffb1112913cb28934433abefcb342c846e7c3c0b0`
records 238037us of receiver admission lock wait, all overlapping optional
preparation. In exchange `09394b5697cec50e1a03021705ef408f3ba9bf992802cc782a1bc7dfb9804174`,
the source's write-to-grant-read span is 428606us while the receiver's complete
request-read-to-grant-write span is 29875us. The source reader remains in its
read phase. These are matching per-process durations, not clock differences;
they do not identify network loss or prove one common cause.

Fresh g349 collection `63df4194a1e6dc837467b27d312f0368` has two matching
successful ordinary handoffs. Source grant waits are 134261us and 55780us;
receiver request processing is 25794us and 28623us respectively. Both services
and input health remain READY. No new fault/reconnect follows connection
stabilization at 10:24:10 UTC; earlier B network-acquisition timeouts belong
to the rollout interval. No g349 lock/PIN trial is present in this collection.

The serial frame reader previously retained no evidence about the dispatch
immediately preceding a request. Optional receiver-only `readPredecessor` now
records that frame's fixed kind and three bounded local spans: dispatch,
dispatch-end to next read-start, and next read duration. It is attached once to
the matching request profile, with no payload, input identity or wall clock.
A first frame stays unobserved. A deterministic stream test models a slow prior
dispatch while the following grant remains fast, preserving abort and EOF.
This adds evidence for the next latency investigation; it does not establish
that a prior dispatch caused either real-device sample or reduce the delay.
Input ordering, lease authority, preparation lifetime and cancellation stay
unchanged. The g349 pair was preserved while the failure evidence below was
collected; subsequent rollout verification must be recorded separately.

Local validation: handoff, peer, productlog, localapi and residentagent package
tests and vet pass. Export tests reject foreign roles, unsupported frame kinds
and out-of-bound durations, and project away unknown payload/timestamp fields.
Read-only review found no change to ordering or failure outcomes. This patch
has not been deployed or device-validated.

A later paired collection `8ee609eea5622660b8541c731e9503dd` supersedes the
initial quiet g349 observation: B first triggers heartbeat timeout on connection
`e7c419e9c739d6ca3dddd91698e0adcb8476e5598c5c649f98f325aa94a1aea7`
at 10:36:12.658713 UTC; A observes remote EOF on that same connection. Further
connections also time out. Both service receipts remain READY with clean source
retirements. This is a new failure investigation, not a passed g349 lock/PIN
trial; the earlier normal handoffs cannot establish continued stability.

## g349: Reject wrong-mode early reports on every outcome

A focused regression reproduces a missing source cleanup fence when an early
KRR1 injection report carries an ordinary failure instead of success. Every
early report without the source summary now adds ErrRecovery and preserves the
reported error, regardless of its status. A valid source report keeps its
existing handling. This is a static protocol-boundary finding, not an observed
cause of the paired g347 failure. Generation 348's successful lock/PIN trial
was collected before applying the locally verified g349 artifact.

The same review reproduces a second ordering: after readiness, a clean final
source retirement can arrive before sourceStart is sent or while its ACK is
pending. Previously both cases became generic authority failure. Only this
input-free startup handshake now preserves the authenticated reader's completed
clean retirement as a terminal transition, never successful readiness. Native
owners must still be joined before replacement. Explicit rejected/foreign ACKs,
missing proof, cleanup failure, emergency stop and input activation retain their
failure. Full authority checks still precede this classification. Stream tests
cover both orderings and prove no input epoch or replay is introduced. This
finding is reproduced in tests and is not yet a device-trial result.

Rollout verification: both PCs now report g349 revision
`43a43496d0bd3bd4db7b4d54bd263ba841ace476`. B transfer
`c0fab239cb7b28f97a93f994c6c4a5cc` reached UPDATED before A was replaced.
Paired collection `79785aa1707a3c26591a596a168b9afe` records successful install
receipts and READY input/service states on both PCs. A doctor is healthy,
CONNECTED, PROTECTED and CONFIGURED; peer version query is ONLINE/IDLE with the
same revision. No new fault is recorded in this initial collection. The g349
physical lock/PIN trial remains pending; no source code or authority settings
changed during rollout.

## g348: Preserve source retirement before startup readiness

Paired g347 collection `fe3ca2b31a96817da2fd8b4b063f8f44` shows B accepting
the first idle source refresh at `2026-10-19T09:32:42.6799334Z`, then failing
source refresh at `09:32:43.2330284Z` during another desktop transition. B's
source-hook fault precedes A's remote EOF; both service receipts remain READY.
There is no cursor-readback failure in this trial. The old startup error path
does not retain enough detail to prove the underlying native failure.

Inspection found two startup retirement races. A source child may finish and
send its authenticated final KRR1 report before sending readiness. The service
previously decoded it only as a readiness envelope, discarded its report and
returned generic authority failure. Source bootstrap now distinguishes a full
final report from readiness, applies the existing receipt cleanup fence, retains
the source exit and original failure, and never publishes readiness for that
child. Missing, truncated, invalid or wrong-mode reports cannot certify cleanup.
The pinned worker/job still joins before the service rebinds its endpoint.

On the agent's independently authenticated pipe, a complete clean retirement
packet is terminal: it no longer requires clearing the live-channel deadline
after the service may already have closed that channel. Failed startup,
cancellation and expired startup budgets still fail. No input is replayed and
no final report or retirement packet grants input authority. Remaining startup
failures now record a fixed stage and classified native error; clean early
retirement has its own fixed event. There are no key identities or input values.

Focused stream tests cover pre-ready retirement, cleanup failure, malformed and
wrong-mode reports, and readiness followed by an independently decoded report.
These reproduce the code-level races; the recorded g347 incident still needs a
new device trial to establish whether either race caused that specific failure.

Device verification: the user reported completion of B lock, PIN-screen
transition, A-to-B control/round trips and login/round trips. Fresh paired
collection `e5f5327ed3f74e9a2bea2ebfb55cc01a` retains g348 revision
`e7c8c7798c2945838a6cae6db55354e0301c7bf0` on both PCs. B records its lock and
desktop transitions at `2026-10-19T10:12:29.0767538Z` through
`10:12:37.0961138Z`, with four clean source exits: capture/sender/interrupt
joined, retirement clean_sent and report ok. Neither side records a new fault,
session failure or reconnect in this trial. Both service lifetimes remain
unchanged with READY input/service receipts, and A doctor is healthy,
CONNECTED, PROTECTED and CONFIGURED after collection.

The collection has 34 matching completed handoff profiles per side and one
earlier incomplete profile per side at approximately 10:10 UTC. That incomplete
sample is not counted as a completed handoff. Previous UI logs are incomplete;
agent logs in this collection are not marked incomplete. The successful device
trial supports continuity for this run, not a proof of the original native
cause or removal of every latency source. No PIN values or input content are
recorded. This device-trial result belongs to g348, not the later g349 rollout.

## g347: Reauthorize one failed cursor observation without replaying movement

Paired g346 collection `ca7047624f7632d90cb1ebcebb9a81cf` records B's first
input failure at `2026-10-19T09:18:06.2785836Z`: `cursor_readback` after dispatch,
followed by `fault_injection_outcome_unknown`. Source deactivation failure
follows this observation and is not the first fault. Both services remain READY.
B's earlier lock/desktop transitions are recorded; a later desktop notification
arrives after the fault. A desktop race is plausible, but the failed GetCursorPos
native code was erased by the old wrapper, so it is not proven for that sample.

The native cursor query now preserves its original error and marks only a real
read-only GetCursorPos failure as `ErrCursorObservationUnavailable`. Cancellation,
failed movement and unclassified adapter failures do not gain that marker.
The installed input worker supplies a private in-process observation executor.
On a marked failed readback it may recheck the current desktop and agent authority
on the same windowless OS thread and read the cursor once more. The interactive
desktop binding retains its existing fixed-name selection, cleanup fence and
100ms preparation budget, within the original command context and lease.
The fixed-desktop runner cannot follow a desktop it was not allowed to select.

Only a successful second observation supplies coordinates to the existing ACK.
The movement, key/button input, placement and entry transaction are never
repeated. Reauthorization/cleanup/cancellation failure or another failed read
preserves the original observation error and the unknown post-move outcome.
No success ACK is invented, no wire schema or failure certainty changes, and no
new handle, desktop selector or authority arrives from the peer. The additional
scope/query work is included in that operation's readback timing.

Focused tests cover recovery with exactly one movement and two reads, persistent
failure, rejected authority, failed cleanup, cancellation, absent recovery scope,
unclassified errors and native error preservation. Existing unknown-outcome and
ACK-framing tests remain applicable. Real lock/PIN continuity must be checked
after rollout; this change does not claim to remove the independent cold-entry
preparation wait observed in g346.

## g346: Refresh an aging candidate on a fresh hint

The g345 trial has an expired-candidate entry taking 174.220ms at the source,
including 158.838ms receiver readiness wait. A preparation hint was acknowledged
between the preceding clean return and entry. A recent reentry candidate can be
kept by the previous 1.5-second refresh threshold, then expire during a slower
physical approach. The fixed acknowledgement does not promise lease readiness.

A fresh authenticated, rate-admitted hint may now replace a candidate aged at
least one second, half its existing lifetime. A replacement gets its own timer
only after the original child's cleanup succeeds. The two-second hint-launch
cooldown, two-second unbound lifetime, single candidate, one claim, independent
lease/bootstrap authentication and ordinary cold fallback remain unchanged.
Recent candidates are retained. No idle timer creates work and duplicates cannot
renew an existing child. This trades a possible earlier replacement within the
existing launch budget for more approach time before expiry; device benefit
still needs paired evidence. It does not address construction already overlapping
an entry. A deterministic replay covers the aging candidate, slow approach and
unchanged cooldown; native ownership/expiry tests cover the adjacent boundary.

## g340: Renew a retreated approach before its edge band

The paired g339 trial completed twelve handoffs. One receiver admission spent
107.828ms in OpenLock while an already-started preparation completed; its total
authorization was 131.007ms. This is a wait for optional construction, not evidence
that the trust check itself took that time. Preparation remains joined so entry
never bypasses native cleanup or discards useful in-flight construction.

After a real retreat and expiry of the existing 2.1-second source cooldown,
the next outward physical movement may send a fresh preparation hint before
re-entering the predictive edge band. Continuous movement previously lacked
the idle-resume exception and could start preparation too close to entry.
The single-slot queue, receiver/sender rate limits, freshness and one-claim
worker expiry remain unchanged. A full queue does not consume the renewal;
inward motion, missing history and an approach without retreat do not use it.
No timer starts preparation and no lease authority is carried by the hint.
The device latency effect remains to be measured after deployment.

## g161: Preview the approach from recent pointer positions

g160's first/idle attempts showed grants of 79–134ms with a preparation-mutex wait
(OpenLock) of 52–105ms, so a hint emitted at the last moment still overlapped the
handoff. g161 predicts the approach earlier from recent physical pointer X positions
rather than raw mouse counts, which acceleration and DPI can change. The estimate is
local and serialized with O(1) work per move: a baseline band of 192px clamped to a
quarter of the width, projected about 150ms ahead and capped at three quarters of the
width. Samples closer than 4ms are coalesced, the anchor advances no more often than every 16ms,
and the estimate resets on a sample older than 50ms, a reversed clock or direction, or
a position outside the display. The sticky approach threshold is kept until an actual
retreat beyond band plus the configured rearm (capped at width-2), so a falling speed
estimate cannot rearm the same outward approach.

No coordinates are logged or sent and there are no new wire or service changes. The
existing single-slot timestamp queue, 500ms freshness, 2s rate limit, 2s unbound
one-claim same-console lifetime, independent authorization/bootstrap and 300ms local
wait are unchanged. These are conservative tuning constants and the hardware effect is
not yet verified. Tests cover both directions, slow or raw-counts-only input,
stale/reversed/outside/duplicate samples, sticky rearm, narrow-screen reachable reset
and the existing session integration. No measured speedup is claimed; g161 real-device
cold latency, continuous jitter and PIN regression remain pending.

## g160: Near-boundary preparation hint over negotiated ALPN

When the pointer moves outward within 192px of the crossing edge, the source hook
enqueues one timestamp hint into a non-blocking single-slot queue (the band is
clamped to a quarter of the screen width). The flag is not re-armed until the
pointer retreats beyond twice the band, and the hint carries no identifiers or
geometry. Both product endpoints advertise TLS ALPN `krasis/1.prepare` with the
legacy `krasis/1` fallback, so only negotiated peers exchange it: the control
command is version 4 kind 7 with no identifiers or geometry, while ordinary lease
commands remain version 3. A sender checks local run state before writing.

The receiver accepts one hint, drops it after 500ms, enforces a 2s cooldown and a
300ms local preparation wait, and requires the existing fresh trust, role and
inbound permission check. The installed agent and service still authenticate the
existing exact path, images, SCM and pipe owner. The fixed `KRP1` local packet
carries zero authority; at most one unbound child is retained for 2s in the same
console session and claimed once, and no input transfers before a fresh normal
lease is approved. Duplicates do not extend expiry, slow/expired/missed requests
use the existing cold fallback, and a failed cleanup remains a service failure.

Existing `preparedAgeMicros`/`prepared` diagnostics are reused and the hint adds a
fixed acknowledged/missed outcome; an acknowledgement is not lease readiness.
g159's first cold handoff was about 132ms and g160's goal is only to move cold
preparation off the handoff. There is no real-device evidence yet, and no latency
improvement is claimed.

## g159: Prepare one unbound child after clean recovery

After a successful lease recovery and owned job cleanup, the installed service
may prepare one new child for the next lease. Its endpoint exists before this
preparation. The child owns only a SYSTEM-only control pipe and independent
installation checks; no agent image handle, input pipe or lease is transferred.
It expires after two seconds and can be claimed only once in the same active
console session. Expiry cannot close a claimed child. Cleanup failure stops the
service instead of authorizing another lease. Missing, expired or exited
preparation uses the normal fresh launch before any input is sent.

The approved new lease still passes agent authentication, exact child pipe-owner
checks, KRA1 service authentication and KRB2 agent-image binding. A new child
consumes at most one lease; an active input worker is never recycled. KRD4 adds
bounded preparation age to the internal diagnostic receipt. Its earlier launch
and child-connect timings describe preparation outside the current handoff;
ReadyWait remains the current lease's measured wait. External peer protocol and
CLI behavior are unchanged. Real-device latency and PIN regression are pending.

## g138: Show portable fallback after updates

The status API and desktop snapshot expose the runner's selected input backend.
Portable selection shows a persistent PIN/administrator-window setup notice
and links to installation instructions in the device update dialog. Installing
the matching protected images requires attended Windows elevation after safe
shutdown. Updating the app alone does not perform that installation.

The observation is fenced to the current session and cleared at completion.
Missing or unfamiliar values remain unknown, including older agents. It is
neither permission to inject nor proof of worker lease readiness or PIN control.
Image matching, installed-agent selection and input authorization are unchanged.

## g137: Distinguish portable protection causes

The portable session guard records one fixed cause code per runner, from the
same observation that detected protection:

- `protected_input_desktop_unavailable`: the input desktop could not be opened
  or its name could not be queried.
- `protected_input_non_default_desktop`: the queried desktop is not Default.
- `protected_input_credential_broker`: the foreground credential broker was
  positively identified.
- `protected_input_identity_unknown`: foreground process identity could not
  be established while its window was still foreground, or its PID was unknown.

These codes survive sanitized support export; no process path, window title,
credential or input content is recorded. The first cause remains stable even
if monitor and input checks race or the foreground changes before cancellation.
Later checks still inspect current state rather than caching a protection
decision. Unknown identity still stops the portable session. The installed
worker path and recovery policy are unchanged.

Older `protected_input_detected` records alone cannot distinguish these causes.
New real-device evidence is needed before attributing a disconnect to a
credential dialog or treating unknown identity as a false positive.

The g104 local Esc probe succeeded with the required desktop access mask. That
did not enable product A-to-B input: portable agents still injected at Medium
integrity and stopped sessions when a credential prompt was observed.

g107 connects the product path. After an attended elevated installer has placed
the fixed agent and input service, the UI prefers that agent only when its
SHA-256 matches the verified sibling bundle. Protected images remain pinned
through process creation. Missing, invalid or older installations retain the
portable path and its conservative protected-input guard.

Only the installed ordinary console agent can select privileged injection.
Its actual process path, token session and administrator-owned image digest
are checked. The SYSTEM service independently authenticates the kernel pipe
client and the exact installed image. Service readiness alone is not permission
to inject: the resident agent first approves the authenticated mTLS peer's
inbound lease, then creates an unpredictable binding for that exact identity.

The immutable delegate sends press, release, motion and wheel events through
the authenticated worker. Local cursor entry and return hysteresis stay in the
existing Windows adapter, including held-button suppression. A lost reply is
never replayed or sent through ordinary SendInput. Worker-owned crash recovery
replaces the ordinary crash guard's duplicate ledger for delegated presses.

While this installed path is selected, a credential prompt no longer cancels
the product connection. A worker heartbeat runs every 250 ms, bound to the
approved session context. Failure cancels the connection. Worker lifetime is
limited by certificate expiry and 55 minutes, below the one-hour protocol cap.
Matching return, lease abort or session cleanup closes the worker; stale returns
cannot close a current lease. Revocation remains enforced by resident trust
checks and context cancellation. No input payload, PIN, window text or clipboard
content is logged.

Automatic update still transports the installer without executing it. Installing
or replacing protected binaries requires the receiving user's Windows elevation
consent. Starting g107 alone does not install the service. After installation,
stop the portable agent safely while disconnected and let g107 start the matching
installed agent. A new revision requires matching installed binaries; an older
service is never silently trusted for a newer portable agent.

Focused tests use fake injection and authenticated in-memory channels; they do
not enter credentials, elevate, install a service or demonstrate remote PIN
success. Real two-PC mouse, keyboard, Esc, return and release-recovery checks
remain required, especially cursor operations while a security dialog is open.

## g108: Ordinary-agent service authentication

The receiving service still checks the ordinary agent's actual token. In the
opposite direction, the ordinary agent no longer reads the SYSTEM service's
token or requests process synchronization access. B's earlier Deskflow token
comparison reported access denied from ordinary privilege; product bootstrap
must not assume that cross-account token query is available.

The service client reads the named pipe's kernel server PID and requires that
PID to be the running fixed own-process LocalSystem service. It verifies SCM
configuration and its administrator-owned DACL, the fixed quoted image command,
session 0, actual process image path, expected image digest and protected file
ownership. A retained limited-query process handle checks GetExitCodeProcess,
without PID lookup on subsequent requests. The pinned service itself checks
SYSTEM/System integrity and restricted/AppContainer state before readiness.
No service/process/token ACL or Windows security policy is changed.

References: [process access rights](https://learn.microsoft.com/en-us/windows/win32/procthread/process-security-and-access-rights)
and [service access rights](https://learn.microsoft.com/en-us/windows/win32/services/service-security-and-access-rights).

## g139: Bounded startup diagnostics

The matched installed service and console child return a fixed 40-byte KRD2
readiness envelope carrying eight bounded duration fields and a presence mask.
The agent records validated detail only in its active installed receiver
handoff profile. It accepts legacy eight-byte KRD1 readiness with no startup
detail. Old readers do not accept KRD2; deployment still requires matched
protected binaries and the existing image/installation checks.

The envelope is diagnostic data after process/image authentication and desktop
readiness, not an authorization claim. Malformed readiness retains protocol
failure handling; invalid numeric diagnostics are consumed and discarded.
Input requests, ACKs, bootstrap lease metadata, pressed-state recovery,
authority checks, worker lifetime and service backoff are unchanged.
See the [startup spans and trial](../experiments/handoff-profile.md).

## g140: Attended protected input refresh in the app lifecycle

After an authorized peer update or idle local agent replacement, startup checks
the fixed protected installation before launching the new agent. A verified
installed agent with a different digest triggers the matching bundled embedded
installer through Windows `runas`. Initial installation requires the local
app button. A matching image with an unready service, or an unverified existing
installation, does not trigger automatic repair. The ordinary UI never gains
privileged input or accepts executable paths from renderer/peer arguments.

The app verifies bundled image checksums and clean VCS revisions, pins files
against writes/replacement across consent, waits for a terminal installer exit,
and independently verifies the installed agent and service readiness. This is
integrity verification for an attended development build, not publisher trust.
Windows administrator consent is still required; no signing key, certificate,
security policy or unattended privileged update authority is introduced.

The local install/refresh button serializes with connection recovery and peer
update application, disconnects input, stops the exact idle agent through the
existing instance-bound API and waits for its single-instance lease to end.
After terminal success/cancellation/failure it starts a verified matching agent
and requests reconnection only to the saved still-trusted fingerprint. A pending
installer outcome fences further installation and agent launch. Transaction
recovery remains owned by the installer; no journal is deleted by the UI.
Cancellation/failure never repeats the prompt through polling or recovery.

## g210: Refresh an existing installation on a new local app launch

The native lifecycle preflights the bundled images and authenticated agent
metadata once at launch. An older agent, or a matching agent with an existing
protected installation requiring an update, enters the same serialized local
maintenance transaction as the button: remember the uniquely trusted peer,
disconnect safely, wait for the exact agent's ownership to end, run the bundled
installer once when required, and launch the current agent. Reconnect rechecks
the saved alias and fingerprint. Initial optional installation, pairing, newer
or incompatible agents and unverified bundles are not automatic maintenance.

Administrator consent is retained. Terminal cancellation/failure restores
ordinary input without repeating consent; a pending installer fences restart
and reconnect. Renderer polling and automatic recovery do not invoke this
launch-only transaction. Tests cover connected old/same-version agents,
consent cancellation, failure, pending ownership, trust change and cancellation
of reconnect. Two-device update/consent behavior requires an attended trial.

Opening a verified newer local EXE while the old UI remains open now requests
a safe disconnect before the existing agent-side release claim and UI handoff.
The replacement arguments carry the saved peer fingerprint; both build and
current trust are rechecked before reconnect. A failed launch releases the
claim and restores only that identity. Pairing/other native operations wait,
and terminal handoff errors stop the request rather than repeating disconnects.
Unattended public release checks remain idle-only. The first upgrade from an
older UI that lacks this behavior still uses that UI's idle-only handoff;
subsequent launches from g210 use the explicit replacement transaction.

The native UI publishes bounded installation-state codes independently of the
selected session input backend. A ready installation is not proof of a working
PIN session. Local tests cover ownership, cancellation, pending results, image
pins and reconnect identity; real two-device consent/update/PIN behavior remains
to be checked on the final g141 bundle. g141 also observes the installer's
existing exclusive transaction lock without creating or deleting it, fencing
installation and agent startup after reopening an app during setup.

## g173: Cursor entry follows the selected protected backend

The g172 operator reports working Esc/click in a B PIN dialog but a disconnect
after returning to A and entering B again while the dialog remains open. In the
2026-10-05 exports (A 12:20:50Z, B 12:20:47Z), exact revision
`a355d82b998e6f7b21e39270c96c6a03c78c0491` has four B entry injection failures
with outcome unknown and four control-dispatch reconnects. A has four remote EOF
reconnects. The diagnostics identify the entry stage, not the failing native
cursor call. Source inspection finds ordinary GetCursorPos/SetCursorPos still
used by the protected adapter's entry path.

g173 routes cursor snapshots, entry writes, verification and rollback through
the selected lease-bound worker under its existing desktop scope. A lost write
ACK retains an unverified pending transaction; an explicitly not-applied write
clears it. Backend/capability errors never fall back to ordinary cursor APIs.
Relative motion returns its position in the same ACK for return tracking.
Portable input keeps its local cursor APIs. The native worker port does not
need a separate combined method: its server performs move and snapshot in one
scope without another IPC request.

Agent and installed worker images must match through the existing attended
refresh lifecycle. No installer, ACL, certificate, policy or consent behavior
changes. Hosted regression checks cover delegation, repeated entry/rollback,
lost ACK certainty, binding, bounds, cancellation and single-roundtrip motion.
The operator reported success after the requested PIN-open B-A-B g173 trial.
The matching exports contain no post-connect entry failure or reconnect in
their short observation windows. See the [g173 trial observations](../experiments/g173-pin-reentry-observations.md)
for the evidence boundary and remaining transition-delay work.

## g175: Prepared worker selection evidence

The service reports a bounded candidate selection code in the optional startup
diagnostic. It preserves the existing claim, expiry, cleanup and cold-launch
fallback policy; a distinct atomic expired marker provides a reason without
reading disposed native handles. KRD5 carries the service decision after
verified readiness, with legacy receipts still readable and matched installed
images required. Child timings cannot replace the selection code. No peer or
input ACK protocol changes. See the [selection diagnostic contract](../experiments/prepared-worker-selection.md).

## g176: Cursor transaction failure stages

Entry and rollback failures now export a fixed operation-specific stage in
their existing injection receipt. The wrapper preserves original error chains
and unknown input outcomes; it does not replay input, change recovery or select
another backend. Stage validation applies to local and peer diagnostics without
exporting cursor positions or native error text. See the [stage contract](../experiments/cursor-transaction-failure-stages.md).

## g177: Recover cursor before retiring its worker

Receiver entry-failure and prepared-abort cleanup now defer matching return
disarm until runtime release and cursor rollback finish. Installed disarm also
closes the cursor backend; early retirement prevented rollback inspection.
Bounded cancellation-independent cleanup, lease matching and error aggregation
remain intact, with unconditional retirement on failure. See the [cleanup lifetime correction](../experiments/cursor-rollback-before-retirement.md).

## g178: Bounded worker apply decomposition

Tracked installed receiver inputs can request two worker-local duration parts
within the existing input scope: application and cursor snapshot. A bounded
diagnostic flag/ACK extension carries only numeric timing, with original worker
timings retained and legacy traffic unchanged. Measurements do not alter input
order, authority, cursor outcomes or recovery. See the [split measurement contract](../experiments/worker-apply-split.md).

## Interrupted image replacement recovery

After a clean service stop, replacing the active image does not replace its
last-run receipt. Rollback selects the stopped generation from a clean protected
service record and independently verified active/retired/backup image pair.
Retired or backup selection additionally requires a valid protected transaction
journal whose previous or target image pair matches. Unknown, starting, ready,
failed or unrelated receipts cannot authorize restoration. SCM stop rechecks
state, process identity and the clean record before any file restoration.

## Service startup after reboot

Successful installation enables SCM automatic start only after the new embedded
service reaches durable commit. Legacy manual-start configuration remains valid
for migration and rollback; ordinary users retain query-only service permissions.
Service startup verifies the protected journal and pinned image pair before
running input. Boot accepts a journal-free installation, a committed target, or
a restored previous image in rollback-start/commit. An uncommitted install-start
target additionally requires the installer's fixed SCM argument. Other phases,
unexplained staging artifacts and malformed journals refuse startup. A committed
cleanly stopped target can regain readiness during installer recovery. Agent
startup briefly waits for an already-starting service without starting or
repairing it. Physical reboot and PIN checks remain separate device verification.

## Attended service recovery without reboot

The explicit recovery button uses the verified embedded installer `--repair`
action after the existing lifecycle has paused the connection and confirmed the
ordinary agent's exit. Before any service restart, the installer saves an
immutable request-scoped sanitized diagnostic snapshot; a save failure stops
recovery. Startup and polling do not invoke this action.

Under the installer transaction lock, a failed stopped service may be started
once only when no journal remains, the active protected image pair and SCM
configuration verify, and the failed record's PID has exited. A live or reused
PID, access-denied query, missing/mismatched record, or unresolved transaction
refuses this recovery. The verified service must publish readiness, then stop
with its own clean receipt and confirmed kernel process exit before the normal
installation transaction can replace files. Failure at any step stops the
attempt and retains the operation and available Win32 cause in the receipt.
No failed record is relabeled clean by the installer. The lifecycle restores
the agent and reconnects only the same trusted peer under its existing rules.

## Planned lease retirement

Ten seconds before the immutable worker lease deadline, a native session task
requests an exact session/epoch/lease return. Runtime serialization releases held
inputs and sends the normal peer revoke before closing the worker channel. The
transport connection remains available for a fresh boundary grant. Release or
revoke failures still fault; expired operations and authority failures are never
replayed or renewed. Device verification remains pending.

Source relative frames share the worker's +/-32767 delta bound. Pending moves
coalesce only within that bound; larger individual moves are split with total
movement and button ordering preserved. Admission reserves the complete split
or takes the existing overflow safety path.
