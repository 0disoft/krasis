# g305 idle source heartbeat retirement

After A rebooted, paired collection `35e5b69e3c691d171cc406cded3becf9`
confirmed both g304 input services READY. A's current boot nevertheless contains
`installed_source_failure_response` and `installed_source_heartbeat_failed` at
2026-10-15T14:49:49Z, followed by a source-hook fault and reconnect. Earlier
heartbeat windows also contain response maxima of ge1s. The native report proves
capture, sender and Interrupt joined; it reports failed final notification after
the agent closed its channel. This localizes the observed failure to response
waiting, not the underlying reason for the delay.

g305 uses the existing exact-request drain and authenticated stop/retirement path
for a fully written heartbeat only while source epoch is zero. Heartbeat has no
native input effect. Its request deadline is unchanged. At timeout it clears
authority, rejects subsequent commands, retains only that pending request, and
notifies the owner. A bounded cleanup consumes only the matching late response
or independent authenticated retirement proof, then requests native stop. The
resident can rebuild an idle source only after that proof, while retaining the
network session. It still rejects active source state or physical presses that
race retirement.

Partial writes, authority loss, wrong responses, missing replies, failed cleanup,
and active-epoch heartbeat timeouts keep their existing fatal/fenced behavior.
No heartbeat/request replay, deadline extension, input retry, generic error
normalization or security-policy relaxation is introduced. Injection IPC is not
changed. The prior source-window native-error preservation fix is also included
in this build.

The new idle-heartbeat exact-drain regression test fails on g304's close-on-
timeout path, then passes with the guarded policy. Tests additionally cover active
heartbeat timeout, missing retirement proof, foreign replies, no new activation
while draining, and exact monotonically sequenced stop without replay. Physical
device continuity and stutter improvement remain unverified until a new trial.
