# Source dispatch and first-entry preparation follow-up

The g307 paired collection `fa078fc582c4a9ed7e9d087aa1e508f7` contains 12
completed matching handoffs. A's CaptureActivate median is 13121.5us, maximum
99675us; its last three spans are 46694, 84200 and 99675us. First network write-
coordinator waits on those attempts are 32105, 65011 and 73151us. These overlap
the activation path and must not be summed. Aggregate source timings do not
identify the worker's internal cause for any one tail sample.

The worker selected authenticated commands and buffered idle pointer boundary
notifications with equal select priority. A pending activation could therefore
wait while additional boundary notifications each passed the writer's authority
checks and pipe write. Dispatch now checks for one pending command first. A
request arriving after the check can still wait for one chosen notification.
Each event queue retains its existing ordering and capacity; no input or boundary
sample is discarded/coalesced. Physical remote actions use the unchanged sender.
The command reader still validates binding, sequence and authority before enqueue;
the writer still authenticates and sequences every frame. Cancellation is checked
before taking pending work, and native cleanup/retirement remains unchanged.

Tests enqueue a full 64-item pointer backlog and an activation, verify activation
dispatches first without consuming boundary items, then verify all boundary items
and a local return remain ordered. Cancellation retains queued work unexecuted.
This proves dispatch ordering, not that backlog caused every measured tail or
that device latency has already improved.
