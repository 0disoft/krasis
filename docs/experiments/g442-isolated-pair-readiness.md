# g442: retain installed native-pair readiness time

Paired g441 collection `87bed82e690eb0f953290ea9d5620e24` at
2026-10-22T15:32:17Z/15:32:21Z shows both protected services READY, no source
exits, and no service failure after installation. A live doctor is healthy and
CONNECTED. B's seven network-acquisition timeouts precede the first connection
during the g441 rollout; they are not new connected-session recovery failures.

Seven matched handoffs retain source grant waits of 147.404-331.073ms and B
ReadyWait of 130.283-317.168ms. B's ServiceBootstrapReady is 120.468-147.348ms,
including child work; overlapping spans must not be added. Input-dispatch gaps
can include intentional user idle, so their maxima are not cursor latency.

The installed isolated journal bypasses serveReadyProfileOnDesktop, where the
direct path records WorkerDesktopReady. Its independent input/release child
pair therefore published no corresponding measurement, leaving most bootstrap
wait unattributed. Record elapsed pair startup only after both authenticated
owners join successfully, using the existing WorkerDesktopReady slot. The
measurement begins at pair launch, after handle/preflight preparation, and
excludes subsequent journal validation and final readiness publication. Child
startup is concurrent; this is elapsed time, not the sum of child durations.

The existing production readiness encoder/decoder regression loses the sample
when the recording line is removed, and passes with it present. Independent
100ms/200ms child startup records exactly 200ms. Failure of either role and
caller cancellation retain existing retirement behavior and publish no readiness
measurement. The existing fixed startup payload and measured mask carry the
newly populated slot; no new envelope, authority, timeout, retry, input path,
native process, cache, public API, DB, dependency, runner or hygiene policy.

This improves diagnosis, not input speed. Do not infer that preparation checks
are redundant or suppress recovery from these timings. Next use a g442 paired
handoff to distinguish native-pair startup from remaining bootstrap work before
changing startup behavior. Keep generic source-command-loop recovery and
boundary gate contention separate unless either recurs with concrete evidence.

Passed: inputworker 22.431s, handoff 0.073s, productlog 2.276s, residentagent
8.544s. The portable-pair fixed-owner fixture was excluded to avoid the installed
session; no ownership fence was changed. No race detector or long soak in this
bounded diagnostic change. Paired application follows in the rollout record.
No push.

Paired rollout: frontend 91 tests, native build, 8.03s bundle smoke, eight-file
inventory and offline verification passed; archive 28,385,384 bytes. B update
`ff493482893b9a4a0546329b41470ddd` reached UPDATED. A's first direct installation
committed the new files but exited 5 at finish/commit/file_delete with Win32 5.
After acknowledged safe-stop, the fixed installer's repair exited 0. App repair
registered g442 and the verified replacement UI resumed the saved B connection.
This resolved the current installation; the cleanup error's cause is unproven.
Do not report the first installer invocation as successful.

Collection `e95c0c0e560cb56e25e10ff11f4c52ba` at 15:41:43Z/15:41:48Z confirms
both successful installation receipts and both services/input health READY.
Final doctor is healthy/CONNECTED with all local images matching g442. There is
no g442 handoff profile yet, so the populated native-pair span is regression
verified but has not been observed in a physical screen crossing. B's acquisition
timeouts were during A's replacement, before this final connection.

Next priorities: (1) collect the next ordinary g442 handoff to attribute startup,
(2) investigate the observed installer cleanup access denial before another
direct live installation, (3) retain independent cleanup evidence if the generic
source loop or boundary-gate failure recurs. No further implementation started
after this bounded change. Results:
`results/2026-10-23-g442-isolated-pair-readiness.json`.
