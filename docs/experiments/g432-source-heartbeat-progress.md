# g432: retain source-child heartbeat progress

g431's terminal agent trace cannot observe the separate source child's runtime.
The client authority and write complete under 1ms before a 2s response timeout;
the later agent-query-closed report cannot identify the original blocked stage.
The large observed agent STW starts after the failure. Do not infer a repair
from that trace or extend the response deadline.

Record the serial heartbeat reader's last stage and its lifetime's largest
elapsed bucket: service presence, agent presence, reply gate, reply presence,
reply write, then replied. The largest bucket retains the first stage at that
bucket; it is not an exact-duration ranking. A complete validated command frame
precedes service presence. A last stage of replied means no later heartbeat
reached that point, not proof that its reply was consumed by the client.

Only the serial reader and its synchronous reply update this record. The
independent presence watchdog, ordinary commands, input and retirement frames
cannot overwrite it. Snapshot after joining the reader, before final reporting.
No per-heartbeat logging, disk I/O, timer, goroutine or native query is added.
Existing checks, query bounds, deadlines, sequencing and fencing are retained.

Use the source report's two reserved bytes for fixed stage codes and buckets.
Reports containing progress use local report version 4, with unchanged 32-byte
framing; versions 1-3 retain their old validation. A source without progress
still uses version 3. Unknown/private stages and contradictory buckets reject.
The service persists these bounded fields in LOGIN-RUNTIME.json. Older runtime
documents remain readable. No environment inheritance or raw child trace is
introduced into the privileged worker.

Validation: inputworker PASS (22.171s), logindiag PASS, productlog PASS (2.258s),
residentagent PASS (7.104s), git diff --check PASS. Regressions retain authority
failure stage, distinguish watchdog observations, keep failed-write evidence
through retirement, and round-trip/reject progress reports. The live portable
pair ownership fixture is skipped to avoid conflicting with the installed
session. Packaging and paired application are recorded separately.

The original response stall remains unresolved pending a failure with these
stages. Next: correlate the client failure with source progress under low load;
then make a targeted behavioral repair. Trust freshness and B entry readiness
remain separate follow-ups. No public API, DB, dependency, runner, signing,
firewall or source-of-truth specification change; no remote push/ref mutation.
