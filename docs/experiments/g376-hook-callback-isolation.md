# g376: hook failure fencing and optional timing isolation (#59 progress)

SourceSession.Abort now publishes its terminal fence and first failure using
atomics and a nonblocking channel. It no longer acquires stateMu or sync.Once.
All grant, offer, current-generation and suppression checks already honor that
fence. The ordinary source worker still owns deactivation, sender shutdown and
remote held-input cleanup; no cleanup success is inferred from the fence.

SourceHook's optional timing observer is now behind a bounded relay. Its hook
queue reserves two timestamp slots for each admitted callback, with a separate
raw-input queue, so raw traffic cannot displace a callback completion. Actual
monotonic start/end values are retained; delivery can lag. Overflow, excessive
reentrancy and unavailable consumer samples increment the optional support/v1
hookTimingDroppedTotal process counter. Internal watchdog atomics remain on
the hook path and do not depend on this consumer. The relay owns no input or
native resource; its Close never waits for an arbitrary observer, and one
process-wide gate prevents stuck consumers accumulating on reconnect.

Passed wininput, agent, residentagent and productlog package tests include a
held state mutex during abort, concurrent abort calls, late grant/offer fences,
a blocked observer, 10,000 callback/raw samples with full queues, fixed-depth
nesting, timestamp completion drain, consumer replacement exclusion and
zero-allocation callback handoff. Existing sender/held-input cleanup and
raw/hook idle-versus-mismatch tests pass.

#59 remains open: low-level physical callback still performs immediate active
input-desktop authorization, synchronous return-cursor hiding, physical/queue
state gates and boundary/pipeline observer work. These must be assessed without
caching stale desktop authority, losing the first click, weakening the held
ledger or silently dropping discrete input. Current fixed-desktop authorization
and first-input presentation semantics are retained. This change removes two
waiting paths, not every cause of native callback delay or repeated recovery.

No new control API, DB, dependency, runner or firewall changes. Support/v1 adds
one optional numeric diagnostic field. No push or issue comments. Paired device
application is recorded below after packaging.
