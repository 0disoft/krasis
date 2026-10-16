# g316 source retirement, trust cadence and inbound grant reads

Evidence is paired request `13eec17d77e79ac176b71a8985e08b65` from g315.
A reported source-hook failure at 2026-10-16T08:38:49Z after an idle heartbeat
response timeout and a source-stop authority failure. Its service recorded
joined native/sender/interrupt owners and `clean_sent` retirement. At 08:57:10Z
A reported trust freshness expiry with `trust_expiry_not_started`. Timer
lateness reached 2-3 seconds near that incident. These records locate stalls
but do not identify an OS scheduling or filesystem cause by themselves.

## Source retirement

An idle heartbeat that was completely written may time out just as its worker
finishes. The late heartbeat response can reach the agent before its reader
consumes final retirement, while querying the now-closed inherited pipe owner
already fails. Previously that query closed the stream and discarded proof.

Only a source-stop on an already-retiring owner may now wait for the existing
reader's authenticated final retirement, within the original cleanup deadline.
It writes no command, replays nothing and grants no input authority. EOF,
negative retirement, unrelated reader errors and missing proof still fail
closed. An active heartbeat cannot enter idle retirement.

## Grant authorization

Inbound authorization previously read the certificate/key anchor and then
reread the document in a second locked store access. Certificate, role,
revocation and current inbound permission now come from one shared-lock
snapshot. Every request still rereads and validates the directory, document
and certificate. No ACL or revocation verdict is cached.

## Trust cadence

The trust monitor previously started a new file-reading goroutine only after
its observation loop handled a cadence tick. Observation delay could therefore
prevent validation from starting. One session-owned reader now consumes the
cadence directly and serially, independently of logging/result observation.
Its atomic observation holds the latest actual successful validation time
and any terminal negative result. A negative result stops that reader and
cannot be overwritten by a later success.

Each success must finish before the reader's previous five-second validity
deadline. The next deadline is completion + five seconds, never delivery +
five seconds. A delayed observer can consume a continuous timely validation
chain; an expired chain or stale completion cannot resume the session.
The existing global read slot still bounds uninterruptible OS reads across
canceled sessions. Guard exit, parent certificate deadlines, revocation and
missing/corrupt stores remain terminal. A full process/OS scheduling pause
can still expire validity; this patch does not widen that policy.

Regression coverage includes delayed final retirement after owner-query failure,
absence of retirement proof, current inbound policy/certificate/revocation,
and a six-second observer stall while independent validations continue.
The stalled-observer test also verifies that revocation remains terminal.
Existing late-read, guard-exit, read-slot and certificate-expiry cases remain.

No public API, peer wire, database, dependency, ACL or runner changes. Real
idle-entry, roundtrip and lock/PIN latency observations remain necessary after
deployment; software spans are not end-to-end cursor-display measurements.
