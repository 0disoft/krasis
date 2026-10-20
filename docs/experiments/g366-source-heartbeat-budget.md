# g366: source keepalive reply budget

Paired collection `95a6aea79b878ff8beb7578e1fc6ffa8` on 2026-10-20 preserves
the recurring recovery investigation. A's service remains READY, but the source
reader reports timeouts at 12:24:08Z and 12:28:32Z. Before the latter exit,
the agent records a source heartbeat response timeout, peer heartbeat expiry,
and a timer lateness maximum of 2305 ms. B also records peer heartbeat expiry.
These are observed triggers; the cause of the scheduling delay is unproven.
The original g362 service-stop cause remains unconfirmed.

The source heartbeat client applies a one-second response limit while active
capture already has a two-second owner receive deadline. A scheduling delay
between those limits can unnecessarily invalidate a healthy owner. This is
reproduced locally with a 1200 ms matching reply and a request made 500 ms
after the previous successful request began.

G366 lets only source keepalive replies use up to two seconds, capped by the
original active/cursor owner's remaining receive budget. Standby keepalives
also stop at two seconds, rather than holding capture commands for the full
ten-second standby window. Input/capture commands retain their one-second
limit. The worker deadlines, presence checks, authentication, sequence binding,
native-retirement proof and peer liveness policy are unchanged. Missing active
ACKs still clear the epoch and fence the channel; no late request is replayed.

Tests cover the delayed valid reply in active and standby modes, exact active
deadline expiry, bounded standby retirement and the unchanged activation limit.
Inputworker, residentagent, agent and peer tests and vet pass. Localapi checks
and a generation-scoped build validate the distinct manual trial artifact.
Packaging and installed runtime acceptance must be recorded separately.

This fixes one premature-failure condition, not all recorded recovery triggers.
Delays exceeding the existing active deadline still require safe release.
No public API, DB, dependency, repository-hygiene or CI runner changes. No push,
reboot or installed-program replacement is performed by building this package.
