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

## Verified package and authorized application

The generation 366 bundle is revision
`dc43a1fdbf738470bca4649f451cf9287ae77fae`, 27837707 bytes. All 85 frontend
tests, native build, installer provenance, transfer roundtrip, icon and eight-file
checks pass. Offline update verify returns valid/ok.

The operator explicitly approved application to both PCs. B transfer
`9ce7e4229979334342759158839282c4` completed UPDATED at that exact generation
and revision. A's installer completed successfully at 13:03:15Z, and its
installed agent hash matches the built g366 agent:
`b2761d6da83be177fb5a0acbb9569c6968aacbc2503931beecb0f3d1ec17d439`.
The replacement UI and launch record are verified at the same revision.

Paired collections `abc8460e6ec64af10976d4128f0b6bb6` and
`04246d00c2caad8c2393445d3d2c2c44` preserve initial readiness and the short
post-application observation. At 13:05:07Z both input health and service states
are READY, A doctor is healthy/CONNECTED with matching CLI/UI/running/installed
agent versions, and B reports ONLINE g366 at the same revision. Both logs
contain two handoff profiles and no new fault/session_failed/source-heartbeat
failure codes for this build. Recent source heartbeat response buckets are
below 10 ms on A and below 1 ms on B.

This is roughly one minute of paired observation after reconnect, not proof
that the historical recovery frequency is resolved. Longer normal use and
the previously deferred low-load comparison remain unverified. No remote Git
push, public release or reboot was performed.
