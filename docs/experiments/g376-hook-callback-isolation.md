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

## Paired application and observed recovery

Both PCs run g376 at revision
`ad0e4f032604c5895df394ec8f5097a391199b59`. The 27,841,083-byte package
passed offline verification, frontend's 85 tests, native build, installer
provenance and eight-file bundle checks. B transfer
`5c5203aab732f5d04e3407c16d457d9f` reports UPDATED at the exact revision.

A's first installation returned 4 at 19:34:08Z: replace/file_move, Win32 5,
recovery restored. Read-only doctor confirmed the previous g375 installed
agent and service remained intact. One retry returned 0 at 19:34:42Z and
its installed agent SHA-256 matches the bundle. This is an observed rollback
and retry outcome, not identification of which process denied the move.
No security software was disabled, no process was killed, and no broad
permission/firewall change was made.

Doctor reports healthy/CONNECTED and matching CLI/UI/running/installed agent
and recovery record at g376. The 19:35:50Z/52Z paired diagnostics show READY
input health and complete current logs on both PCs. Both connected at
19:35:35Z. Neither g376 report contains a fault/session_failed event; no
reconnect follows 19:35:30Z. B's earlier acquisition timeouts occurred while
A was stopped for replacement. This brief idle connection does not verify
physical held-input, first-click, lock/unlock or long-term stability.

#59 stays open; all ten previously open issues remain open. No issue comments
were added. The next bounded implementation should address synchronous native
presentation and physical/boundary gates without weakening desktop authority
or first-input semantics, then finish #59 before claiming hook work complete.
#50's non-cancelable injection lifetime and the actual device/signing gates
remain separate outstanding work. No new task was begun after ten minutes.

All source and evidence changes are local main commits; one checkout, no new
branches and no Git push. The dependency dashboard was left unchanged.
