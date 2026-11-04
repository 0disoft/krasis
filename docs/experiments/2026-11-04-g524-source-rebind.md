# g524: reacquire a source endpoint only after confirmed retirement

The g523 incident collection `84538fa8fe5ce30ec6f36630fad9ecb3` records
A's `source-service-query-deadline-checking`, followed by clean capture/sender/
interrupt joins. The source retirement reached the app at 13:23:06.595Z;
the replacement pipe open failed at 13:23:09.112Z. The service published its
joined source report at 13:23:09.304Z. These are observer timestamps, not a
precise native join timeline, but they expose the early-retirement/late-rebind
window. Both services remained READY. The app latched FAULTED after rebind.

61 of 64 final system-CPU windows were at least 95 percent busy. The last
summary's diagnostic sampler was 875.613ms late. CPU pressure is a plausible
contributor, not proof of which process or Windows call blocked.

## What the timeout actually measures

`authorizeSourceReadProgress` gives the two pinned-owner presence checks a
shared one-second budget. For the service, `checkPresent` calls
`GetNamedPipeServerProcessId`, not `QueryServiceStatusEx`. The query's
`checking` phase includes native-call wall time and resumption delay before
its completion is published. Existing evidence cannot split those components.
The full SCM/desktop proofs for actual capture/input remain independent.

## Change and safety boundary

A CaptureSource arms a one-use rebind allowance only after its authenticated
clean retirement marker survives local reader join and pipe/proof handle close.
The replacement must be inactive with no physical pressed state. It may retry
only a failed **pipe_open**, before any new lease bytes or input are written:
missing/busy pipe or that attempt's expired deadline. Each connection retains
its ordinary two-second open/authentication budget; all retries share a single
10-second receive-only budget and cancellable 50/100/200ms capped backoff.
Initial connections retain the original behavior.

Every successful open still authenticates the fixed service, kernel pipe owner,
SCM state/configuration and pinned image. Access denial, authentication failure,
unknown/joined error causes, cancellation, protocol failure and failed cleanup
never gain retry eligibility. A connection returned after cancellation is
closed before denial. Active input/heartbeat/query deadlines, native-owner
joins, emergency handling, input ordering and no-replay rules do not change.
Only endpoint acquisition is repeated; no input, lease or command is replayed.

Two fixed diagnostic codes distinguish waiting for rebind and successful
reacquisition. No wire layout or input authority changes. Product generation
advances to g524 for paired installation.

## Validation and limits

Virtual-time regression reproduces a service endpoint becoming available after
3 seconds: the old 2-second attempt fails and the replacement succeeds at 3
seconds after one bounded retry. Tests also cover the single shared 10-second
budget, shorter caller deadlines, cancellation, rejected/joined causes, initial
connection behavior and cleanup of late/partial connections. Real pipe rebind
and existing query/retirement tests remain applicable.

This prevents a confirmed, recoverable retirement from immediately becoming a
permanent source fault solely because the service is still publishing its next
endpoint. It does not make expired authority valid or guarantee operation under
unlimited CPU starvation. Unconfirmed cleanup or absence beyond the total budget
still fails closed. Physical post-install CPU-pressure/crossing validation is
separate from these deterministic tests.

Sources: [GetNamedPipeServerProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeserverprocessid).
