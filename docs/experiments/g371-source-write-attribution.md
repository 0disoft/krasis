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
