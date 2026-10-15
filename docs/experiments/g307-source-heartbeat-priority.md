# g307 source command priority over unsent heartbeat

Paired g306 collection `9b0a6eb68a1645d747b6f2d2a40783c1` confirms 11/12 B
receiver profiles used prepared input children, with ReadyWait 15340–19461us.
A's matching CaptureActivate spans remain 14625–267811us. First input write-
coordinator waits reach 77633us. Source timing windows include client_lock and
client_response in the 100ms–1s bucket. These aggregate maxima do not identify
which heartbeat overlapped an individual activation or add up to cursor latency.

The source client serialized heartbeat and commands through their matching ACK.
Heartbeat TryLock skipped an occupied gate but could overtake published command
demand while the gate was free. It also sent every 250ms even after a successful
authenticated command had already provided liveness.

g307 publishes command demand before acquiring the IPC mutex. Unsent maintenance
yields before and after TryLock when any such demand exists. A successful exact
reply records that request's local write-start time; heartbeat sends only when
that request is at least 500ms old. The existing 250ms polling tick, 1s request
limit and worker's 2s receive idle limit remain unchanged. A late response cannot
buy a fresh 500ms interval. Skipped maintenance neither changes the timestamp nor
sequence/epoch/authority; negative or foreign responses cannot refresh it.

Once any request is transmitted, the same mutex, exact reply matching, timeout,
retirement proof and failure fences still apply. Activation cannot cross an
in-flight heartbeat. No pipelining, cancellation/replay, cached authorization,
native desktop change, protocol format change or input retry is introduced.
The unavoidable residual is an activation arriving after heartbeat transmission.
Worker/native handling and peer network write contention are not changed.

The recent-command regression fails against g306's polling implementation and
passes with this policy. Tests cover delayed ACK age, repeated skipped polls,
free-gate published demand, queued activation behind a held heartbeat, exact
sequence and epoch, busy/cancelled polls, foreign reply failure and existing
idle/active heartbeat retirement fences. Real-device latency and lock/PIN
continuity require fresh g307 measurements; g306's observed improvement is not
claimed as g307's performance result.
