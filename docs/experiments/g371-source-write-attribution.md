# g371: preserve the source worker's actual write failure

The g370 paired trial failed at 2026-10-20 18:22:25Z with agent-side reader
EOF and heartbeat failure. The service's independent source exit reported
command OK, reader cancelled, capture/sender/interrupt joined and retirement
delivery failed. Its generic `source-command-loop` cause did not identify
the event that cancelled the reader. No hook-progress cause was observed.

The source worker now retains fixed write classifications through its final
authenticated report: deadline setup, proven unsubmitted write, partial write,
attempted timeout, closed pipe, native failure and other failure. Heartbeat
reply failures retain their command identity. Classification refines only the
selected original transport container, so a later retirement write cannot
replace that failure. Cleanup uncertainty retains its previous classification
and fence. No retry, deadline extension or new input authority is introduced.

Regression tests exercise real write/error wrapping, source finalization and
report encoding/decoding, including a later failing retirement and a separate
native cleanup failure. The inputworker, logindiag and productlog suites pass.
This identifies a diagnostic loss, not the underlying live g370 write cause;
no remaining issue is declared resolved by this change alone.

No public API, DB, dependency or CI runner changes. The private report's
operation vocabulary is append-only and contains no raw error messages,
input, user/session identifiers or addresses. Physical fault matrices,
Authenticode release acceptance and hosted CI remain unverified. No issue
comments or Git push. Paired installation results are recorded separately.

## Paired installation

The eight-file bundle passed native packaging checks and offline verification
at revision `e0defc0d527fb2fa3379491124898a800871e280`, generation 371,
27,880,123 bytes. The package build also passed the UI suite and native asset
build. B transfer `c1db55a45942f5de825836c8aa9e922b` reports UPDATED at
that exact revision. A's installer returned 0; the installed agent SHA-256
matches the bundle. A's CLI, UI, running/installed agents and recovery launch
record all match g371. No permission prompt was added by the agent.

Paired collections at 18:30:01Z/00Z and 18:30:53Z/52Z on 2026-10-20
reported input health and service READY on both PCs, with no g371 fault or
session failure. A doctor is healthy/CONNECTED. The roughly one-minute
window does not establish a fix for recurring recovery or complete #59/#50.
No additional issue was closed; 14 remain open. The next implementation
priority remains callback waiting under #59 and uncancellable native injection
ownership under #50. A new source write failure should first be compared with
the now-preserved worker classification instead of attributing EOF to hooks.

The follow-up report-privacy assertion inspects bytes before the decoder
consumes its buffer. It changes tests only; installed binaries remain the
verified revision above. No product source changed after packaging.
