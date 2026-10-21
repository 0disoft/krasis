# g416 runtime recurrence: investigation remains incomplete

## Observed evidence

Fresh paired collection `afe92d992120d2997f07885456cffb86` at 08:47:22Z
confirms failures on installed source `eb6047375746698ecba7635239f59b53c9c75673`.
It corroborates collection `8ceec1d441abfc3b3edc25aeea23369f`; the short
post-install success recorded earlier is not long-duration acceptance.

- B records `session_trust_freshness_expired` at 08:37:14.8256983Z and
  `fault_trust_freshness_expired` at 08:37:14.8843604Z.
- B's GC trail has one interval from 20321901484 to 20326487550 microseconds
  relative to its diagnostic baseline: a 4.586066-second sampling gap and
  4.486448-second timer lateness. Its stopping and total GC aggregates each
  increment their final bucket once; estimated GC CPU increases 31.162778
  CPU-seconds. Aggregation uses original histogram upper bounds. These values
  do not establish an exact 4.49-second GC pause, a specific blocking syscall,
  or causation of the trust failure. Do not align independent baselines or
  different PCs' wall clocks as an exact runtime timeline.
- A records source authority failures at 08:35:17.2308638Z and
  08:37:15.1895394Z; its source workers report joined capture/sender/interrupt
  and clean retirement. B also records remote EOF at 08:35:16.5725355Z.
  These earlier failures do not have the same long GC sample and must not all
  be attributed to B's later GC observation.
- A's source exit at 08:44:55.9002496Z reports `hook-callback-stalled` with
  capture/sender/interrupt joined and clean retirement. This is additional
  current-build evidence for the still-open native callback investigation.
- B's historical input health reports SERVICE_RECOVERY_REQUIRED (1066), while
  a fresh local doctor reports healthy CONNECTED, running service and matched
  g416 UI/resident/installed revisions. Live connection health does not erase
  earlier service/input failures or prove physical input acceptance.

## Upstream and local comparison

[Go issue 81759](https://github.com/golang/go/issues/81759) describes a syscall
processor-retake race, including a Windows CreatePipe reproducer. Its proposed
[fix PR 81835](https://github.com/golang/go/pull/81835) is still open/unmerged
as checked through the GitHub API. The official
[release history](https://go.dev/doc/devel/release) lists 1.27.3, but the
unmerged fix cannot be assumed present. Local Go and g416 use 1.27.1.
This is a plausible lead, not a confirmed Krasis root cause.

Existing production code already bounds USER message waits and owned process
waits to 50ms, and drains guard stderr through an overlapped pipe. Reapplying
those changes would not fix a newly established defect.

The existing isolated `.cache/stw-message-repro/repro-1271.exe` comparison was
run once: unbounded message waits completed 20 seconds / 44,249 GC cycles
(maximum progress gap 145.8602ms), and bounded waits completed 20 seconds /
54,847 cycles (133.213ms). Neither reproduced a five-second stall. This short
negative result neither rules out the runtime race nor verifies the device
failure fixed. Both owned fixture children were killed and reaped by its
deferred cleanup. The fixture is scratch evidence, not product source.

## Next bounded investigation

Capture an actual failing process's execution trace before changing native
lifetime or trust policy. The existing opt-in flight recorder is disabled for
ordinary/service launches, lasts 15 minutes, and stores local traces excluded
from peer/support export. No new trace was present in A's diagnostics directory.
Use the documented per-launch diagnostic mode, preserve a scratch copy before
offline analysis, and inspect GC stop/syscall/runnable intervals. B needs its
own local trace; A's trace cannot prove B's cause.

Trust freshness remains five seconds. No source fix, toolchain upgrade, new
generation or device replacement is claimed. No issue is closed from these
findings. Signing, attended physical trials and exact-revision hosted CI remain
the separate closure blockers recorded in the open-issue audit. No API/DB,
runner, firewall, certificate or remote-ref change is part of this investigation.
