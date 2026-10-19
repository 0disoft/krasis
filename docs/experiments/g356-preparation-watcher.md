# g356: Wake and join preparation's exact pending read

g354's long movement handoff had no prepared candidate after a nearby preparation
miss. Code inspection finds avoidable work inside preparation receipt completion:
watchPreparationPeer sets a read deadline and joins its disconnected-peer reader,
but native pipe reads observe changed deadlines on a 20ms wait/poll cycle. This
can consume part of the unchanged 300ms agent preparation budget even after
unbound construction has already finished. It does not prove why that particular
preparation missed or account for all cold startup time.

The native watcher now gets its own read context. On successful or failed
construction, first disarm peer-failure cancellation, interrupt its deadline,
cancel this reader and join its return before writing any receipt. The parent
construction context is not canceled by this intentional stop. Peer EOF/extra
bytes still cancel a build that is in progress, and build/cleanup errors remain
errors. Generic connections retain the existing deadline fallback.

Pending native I/O with a cancelable context registers an exact-OVERLAPPED
CancelIoEx callback after submission. The I/O completion event wakes immediately
when cancellation completes; it need not wait for the next polling timeout.
The callback is stopped or joined before the OVERLAPPED/buffer is unpinned or
its event is closed. Pipe close still joins active I/O. Cancellation that loses
to a completed kernel operation preserves the real transferred bytes and result.
Only an actually aborted operation returns the context cancellation. Ordinary
background-context input read/write operations add no callback. Existing source
command context reads also gain prompt exact-read cancellation while preserving
their final duplex writer and next-reader usability.

The preparation budget, process lifetime, authorization, request and ACK framing,
lease expiry, input ordering and unknown-outcome rules do not change. There is
no input replay, whole-pipe cancellation, timer extension or authority cache.

Focused tests verify explicit read-context use, watcher join before return and
preservation of construction failure. A real Windows named-pipe test verifies
that watcher cancellation leaves receipt writing and a later read usable. Existing
native tests retain completed bytes across late cancellation/deadline observation,
deadline recovery, source-read cancellation and concurrent close behavior. The
full inputworker package tests and vet with only the known Win32 unsafe-pointer
analyzer disabled pass. Race instrumentation was not run: no C compiler is
available on this command environment's PATH. No UI or peer protocol changed.
Localapi tests also pass after reserving source generation 356.

This was a separate local change while the g355 attended movement comparison
was pending; at the initial commit generation 356 had not been built or applied.
Keep the
running g355 baseline until its result and fresh paired diagnostics are assessed.
No new installer, reboot, service restart, push, public release or CI run occurred
for this change. Device startup and movement benefit remain unverified.

## Prepared bundle, not applied

Generation 356 is now packaged at source revision
`db25de6f7ecd1305fe8f4a85d608061f9c10e9a1`, which includes `7d634ea` but not
the rejected event-reuse candidate. The ordinary workspace build passed all 85
frontend tests, native UI build, installer provenance, package roundtrip and
eight-file bundle checks. Offline update verification returns valid/ok, generation
356, that exact revision and archive size 27747344 bytes. Agent, residentagent and
inputdesktop tests additionally pass against this source. No live installer or
new app has been run for this generation.

Independent PowerShell SHA-256:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 50373d6411c641141c184facd2fa5619e0259f2f5a2c27b7143d162d082db9f8 |
| krasis-agent.exe | 92b6562ac0f18fef3f9f8fab84df4e2312a7424b174d98044eb84585e1063262 |
| krasis-cli.exe | d1a1dcdfaec506c521b85a17bdb15a542ff6689267bb41760d22479bbf4c7ee5 |
| krasis-install.exe | 1cece25d088ee45683a8390d4a91a2ef4c8a1b101b2c55d242a835dcfebe9aff |

At packaging time both devices still ran g355. The pending user comparison was needed to separate
g355 movement effects from a subsequent g356 startup change. No push, public
release or CI run occurred. Authority and user-visible API contracts are unchanged.

## Paired rollout and baseline, 14:13:02Z

The g355 comparison returned an inconclusive possible regression under concurrent
work on A; collection `85698d6530b363c3fed5c4c440830853` is analyzed in the g355
experiment. Its last 12 paired handoffs expose one unprepared receiver, with
310024us source request-to-grant, but no retained trial fault/session-end trigger.
This supports proceeding with the bounded preparation change without claiming
a measured g355 movement improvement or regression.

Offline bundle verification again returns valid/ok, generation 356, source
revision `db25de6f7ecd1305fe8f4a85d608061f9c10e9a1`, size 27747344 bytes. All four
independent SHA-256 values match the table above. B transfer
`0f0e3ba5faa6b560d3f7759b9b4a803e` reaches UPDATED at that revision. Ordinary A UI
replacement and automatic protected-input refresh then finish; connecting the
existing B profile returns A to healthy/CONNECTED/PROTECTED/CONFIGURED. UI,
agent, installed agent and launch record all match g356; the service is RUNNING
at its expected path. B update inspection reports ONLINE/IDLE at the same build.
No reboot or manual service start was used.

Paired collection `3de6d9c329483574f37af6fb5c4a4ef4` is RECEIVED with both supports
created at 2026-10-19 14:13:02Z. A install receipt time is 14:12:37.7379053Z and
B is 14:12:04.3958474Z; both have result 0, stage complete and recovery not_needed.
Both services and input-health records are READY. The retained exact-build
events (A 8, B 32) have no fault, failed code or session-end trigger. This is a
short rollout baseline, not attended input or long-term stability verification.
Both exports flag the older control.previous.jsonl as incomplete. Automatic
installer process exit and stdout were not captured separately from receipts.

Next observation is the same 15-second idle, about 20 seconds of continuous B
movement and three returns, when A's other work is quiet. Do not combine the
earlier loaded-host feedback into a controlled effect-size claim. Existing local
tests/build evidence remains applicable; only docs changed after packaging, so
no rebuild, broad retest or CI run was performed. No push, public release, API,
DB or runner change occurred. Desktop-app, ops-change and incident-triage review
procedures were used.

## Attended movement observation, 14:18:11Z

The user reports the requested g356 movement trial seems acceptable ("괜찮은듯").
Do not turn this into a quantified comparison or confirmation that all concurrent
host work stopped. Paired collection `7e6830edf66d7d9d8b7632de14ebd6bf` is RECEIVED,
with A support created 2026-10-19 14:18:11Z and B 14:18:10Z.

Filtering to exact source revision yields 12 matched handoff tags on both PCs,
all ending ok. The retained movement profiles are around 14:17:33..14:17:53Z;
all 12 B startup records reuse a prepared worker (preparationDecision 1).
Ordinary dispatch maxima are A 5593us and B 10850us. A request-to-grant spans
30006..50150us across these profiles. These are measured software spans, not
cursor-display latency or proof of a causal improvement over earlier generations.

The longest B profile has 746 cadence intervals with histogram
[0,117,415,194,20,0], i.e. 20 intervals at least 25ms and none at least 100ms.
Cadence includes intentional idle, transport and scheduling, and the user did
not confirm an exact continuous-movement duration, so no like-for-like percentage
comparison to g354/g355 is claimed.

Both service records retain their rollout start times with service READY and
input health READY. A doctor remains healthy/CONNECTED/PROTECTED/CONFIGURED;
peer inspection is g356 ONLINE/IDLE at the matching revision. No exact-build
fault, failed event or session-end trigger is retained. B's six reconnect
acquisition timeouts are at 14:12:19..14:12:55Z, before this movement trial,
during rollout and reconnection, not trial disconnects.

A's 14:18:02Z runtime summary still retains queue residence 40537us, timer
lateness 99027us and a 145006us slow control dispatch. These separate maxima
are not additive and do not establish a GC or competing-process cause. B's
latest retained runtime summary predates movement (14:17:13Z), so it cannot
characterize that trial's runtime scheduling; the handoff records do cover it.
Both exports still flag the older control.previous.jsonl as incomplete.

Keep g356 as the current observation baseline, with no new code or rollout from
this result. Paired diagnostics, doctor and peer-version inspection passed;
git diff --check validates this documentation-only update. No rebuild or broad
retest is needed because source and artifacts are unchanged. Longer ordinary
use and later lock/PIN regression checks remain outside this movement trial.
No API, DB, repository-hygiene or runner change, push, public release or CI run
was made. Desktop-app, ops-change and incident-triage procedures were applied.
