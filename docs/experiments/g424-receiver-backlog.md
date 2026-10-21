# g424: bounded receiver read-ahead and ordered pointer coalescing

The g423 user comparison reports unchanged B stutter. Its paired software
profiles retain long source writes and receiver ACK waits. Inspection finds
that synchronous per-frame dispatch prevents the receiver from combining
pointer movements already queued behind slow installed-input RPC.

First local commit d8fe782 adds the runtime batch primitive. Each original
session/epoch/sequence is checked and consumed under the runtime owner before
one validated pointer injection. Existing liveness admission, failure cleanup,
pressed-state recovery and retired-lease draining remain in the same owner.

Observed production sessions now have one joined reader with a fixed 64-frame
queue. Dispatch combines only immediately available adjacent pointer moves,
up to 64 and within the existing delta bounds. It never waits for future moves.
Key/button/wheel/control/heartbeat frames, malformed input, identity/sequence
changes, overflow and zero displacement stop the batch and retain the barrier
for the next dispatch. No frame is silently dropped, no heartbeat validated
early, and no wire schema or authority check is weakened. Reader shutdown
closes the exact connection and joins before returning; cancellation also
closes its queue so a waiting consumer cannot remain stranded.

The slow-port reproduction feeds 128 moves with a key-down exactly between
the two 64-move halves. With 20ms per injection the unbatched path needs
2.56s of native waits. The regression requires at most five injections/100ms,
unchanged total displacement, exact key position after 64 units, and EOF key-up.
This synthetic timing is not physical-laptop latency or a new support limit.
Other regressions retain sequence/identity rejection, malformed/barrier order,
cancelled held-input recovery and input/heartbeat liveness.

Full application tests pass before reader integration; full peer tests pass
after integration (2.583s). One pre-existing test's expected move-call count
is changed to total displacement because batching intentionally changes calls.
No assertion about liveness, frame order, EOF cause or held-input release is
removed. The initial integration exposed a cancelled-consumer wait, fixed by
closing the owned queue; its unchanged regression then passes.

No issue is closed solely from this synthetic result. Packaging, paired
application and current-device acceptance follow separately. No public wire
schema, DB, dependency, runner, signing or firewall change; no push.

## Verification and paired rollout

The final full peer/agent/residentagent suites pass in 3.972s/2.076s/6.780s.
Targeted real-child input-stall, release-stall and incomplete-retirement
regressions pass on current source in 4.309s. Workspace packaging passes 85 UI
tests, native frontend/Windows and installer builds, archive smoke (9.55s),
and eight-file verification. Offline verification accepts g424 revision
7185702b47ec58825b9fe3b0c9260676b9be2509, 28185873 bytes.

B update 6b8efa78e5800a1aeb439961c7a7a863 reaches UPDATED for the app,
but its protected-input installation receipt is result 4 at
11:32:08.2907661Z: replace/directory_retire_move, Win32 5, previous
installation restored. The app result is not proof the input service upgraded.
Fresh paired collection 347b574b6312f2188a873a9e64fd5354 still reports
B input UPDATE_REQUIRED at 11:34:58Z. B's current UI is opened via the
authenticated app-open operation; a user handoff requests its recovery action,
which the current remote API cannot invoke. Do not claim B protected g424 READY
or a successful physical movement comparison before this is resolved.

A safe-stop and g424 repair complete, install result 0 at
11:33:01.3954165Z. Only the path-verified old g423 UI process is terminated
for replacement; the installed g424 agent and new UI are launched. At 11:34Z
doctor reports healthy CONNECTED with matched g424 UI/agent/CLI and installed
agent, service RUNNING; input health is READY. A's new UI is left open.

Issue #50 is closed at 11:34:04Z against its actual three completion criteria,
with the separate local acceptance record in
results/2026-10-21-issue-50-acceptance.md. No issue comments are posted.
Physical performance, source reconnect/service failures and the full native
matrix remain unresolved; B's service repair and a g424 comparison are next.
No new implementation task started after the ten-minute cutoff. Hosted CI,
signing and the full physical matrix were not run.
