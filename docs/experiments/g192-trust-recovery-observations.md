# g192 paired trust-monitor and recovery observations

- Evidence: A support export created 2026-10-06T15:07:29Z; B support export
  created 2026-10-06T15:07:27Z. A later export preserves the same g192 events.
- Build: `01455b8c7474801b74e55dacb1421e369efb7529` on both agents.
- Scope: exported event history and source inspection; no PIN or input content.
- Completeness: current and previous agent logs are available; the previous
  control log is marked incomplete on both hosts.

## Observed sequence

| A trust freshness expiry (UTC) | A result | B subsequent CONNECTED (UTC) |
| --- | --- | --- |
| 14:36:00.2926412 | session fault; replacement follows | 14:36:13.0753545 |
| 14:37:09.1426162 | source-thread join fault; replacement follows | 14:37:36.8432431 |
| 14:38:54.1525383 | session fault; replacement follows | 14:39:13.1834479 |
| 14:41:16.2190833 | guard-close timeout; session fault | no later CONNECTED in export |

A shows three replacement stop/start pairs after the first three failures. The
user's g192 screenshot reports automatic recovery exhausted. The last local
scheduled-recovery receipt says `agent_present`: it only confirms that a process
is present, not that the input session is healthy or connected.

Trust timing codes identify different active phases: document decode, parent
metadata, directory metadata/ACL, lock-file open and certificate validation.
Actual file-lock acquisition is typically in the `lt1ms` bucket. A revoked or
missing peer is not reported in this sequence. Scheduling, GC and timer delays
also appear in the runtime summaries. These observations do not establish which
external process or filesystem operation caused the delay.

## Source finding and g194 correction

Previously, a trust-read worker was cancelled after the one-second observation
budget. Its completion was discarded even if it could finish within the existing
five-second freshness window. Another tick started the whole read again when
the single-worker slot became free. Repeated slow successful reads could
therefore fail to refresh the session and consume the bounded recovery budget.
This is a reproduced software failure path, not proof that every exported
timeout was caused by that path.

g194 retains and consumes the same pending read before the unchanged hard
deadline. Deterministic tests cover 1.5s and 4s success with no later tick,
overlapping ticks, a slow revocation result, success exactly at or after the hard
deadline, cancellation, certificate expiry and guard exit. Read workers remain
bounded; late results cannot revive stopped sessions. Checks of ACLs, locks,
peer identity and certificate validity are unchanged.

Two-device g194 acceptance is pending. Reads that genuinely exceed the hard
deadline will still stop input. Repeated recovery exhaustion and guard cleanup
under load remain hardware acceptance cases; the recovery budget is not enlarged.
