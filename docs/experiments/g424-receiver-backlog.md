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
