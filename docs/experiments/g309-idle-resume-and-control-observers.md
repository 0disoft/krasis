# Idle-resume preparation and control observer contention

The paired g308 collection `a723cda9221eea20f622f7acdaedc45d` contains 12
matching completed handoffs. A's capture activation spans are 11.534–27.202ms.
The first B entry has no prepared candidate and spends 142.743ms in ReadyWait;
A waits 194.849ms for the grant. Later entries use prepared candidates and
spend 14.701–18.790ms in ReadyWait. One repeated entry still waits 86.954ms
for the grant while B's request-read to grant-written span is 35.216ms.
Those process-local spans do not locate all network or scheduling delay and
must not be added to overlapping spans or called cursor rendering time.

## First outward movement after rest

Previously preparation waited for the edge prediction band, including after a
long pause when velocity samples had expired. After at least 2.1s without valid
physical pointer movement, a new outward movement may now enqueue preparation
before reaching that band. Session construction initializes the activity clock;
there is no timer, periodic launch, synthetic movement or input grant.

A full hook queue allows retry during the next 500ms of physical movement.
Successful enqueue consumes this opportunity and suppresses another hint on
the same approach. Expired retries, reversed clocks, inward movement, keys and
positions outside the configured display cannot trigger this path. Continuous
movement cannot extend the retry window. Sender, receiver and service cooldowns,
the single unbound child, 2s expiry and full lease authentication remain intact.
An immediate crossing can still outrun preparation; a slow approach can outlive
the candidate. This targets the observed cold-entry risk, not guaranteed warmth.

Deterministic tests cover both edges, a busy hint queue, one successful enqueue,
same-approach suppression and expiry without a timer or repeated launch.

## Grant-path observer work

The resident control-flow observer acquired the global Operations mutex for
every notification, including request received, return armed, entry preparation
and grant written. These notifications neither update a notice nor record a
counter in this observer. They now return before the mutex. Events that actually
change a notice or counter retain the lock and current-session/lifecycle checks.
This removes a potential shared-state stall from the grant/read loops without
reordering authenticated frames, changing lease decisions or dropping existing
diagnostic records. The separate handoff collector remains unchanged.

A contention regression test holds Operations.mu and delivers all ignored
events; they must finish before that lock is released. Existing notice/history
tests and a current-versus-retired capture-success test cover retained effects.
This is a statically identified avoidable wait, not proof that it caused the
86.954ms sample. Device remeasurement must determine the remaining grant tail.
