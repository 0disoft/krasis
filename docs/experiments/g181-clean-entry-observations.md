# g181 retained entry and delay observations

Exact product revision: `13b71a0a17cb8b071b5158cab6d58eec4f94a43d`.
The exports retain actual g181 agent events. This is an observational trial,
not a controlled latency comparison or a live-status receipt.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 18:00:39Z | `4AD7F32CE2102386C4386687DF4ABFBEABF3F9ACC250465625D3027E0366C57C` |
| B | 2026-10-05 18:00:35Z | `5BCCCB1D49A1F502B5B5F78D9B4692D74D80B2877DF7C61D5B12DF4CFA0E21DF` |

## Entry and connection evidence

Each host retains twelve complete `ok` profiles with no failed retained attempt.
A's retained profiles are source-side; B's are installed receiver-side. These
are the first twelve retained attempts, not a count of all crossings in the trial.
Their local attempt ordinals are not independently verified wire correlation IDs.
B has 113 lease-ready events in this build. Frame/histogram conservation passes:
A 1,115 [1108,6,1,0,0]; B 1,099 [70,1022,7,0,0].

A records connection at 17:59:06.328595Z; B separately records connection at
17:59:06.1677146Z. Neither has a subsequent failure/reconnect event before export:
about 92.7 seconds of event history on A and 88.8 seconds on B. B's four earlier
acquisition timeouts, one reconnect-state event and a requested disconnect precede
that connection; they are not post-connection failures. Their initiating context
is not established by this export.

There is no g181 `injection_failure`, entry/mismatch, installed-input connection
failure or heartbeat failure. No mismatch observation is exported. Because g181
logs comparison flags only with an entry/mismatch failure, absence of flags does
not demonstrate that a changed-constraint or physical-coordinate branch executed.
The g180 mismatch cause remains unclassified. PIN-open reentry context and actual
PIN-window behavior require the operator's observation; exports do not retain
window identities or input content.

Each export has one incomplete historical log, with no current-build events in
that log. Current-build profiles are complete. Short clean history cannot establish
long-term stability or eliminate intermittent blocked entry.

## Remaining delay evidence

A's twelve retained RequestToGrant spans have min/median/max
27.211/31.2755/120.878ms. Its slowest local attempt (6) has a complete grant decoder
receipt: 116 bytes, one frame, two reads, 120.878ms before initial byte progress
and measured zero in subsequent reads/decoding. This local wait includes remote
handling, transport and reader scheduling; it does not identify their individual
costs. The same attempt's capture activation is 6.639ms. Source deactivation
49.335ms occurs in a different local attempt (10); do not combine maxima.

B's twelve EntryPrepare spans are 3.730/4.582/5.313ms min/median/max; its prepared
worker is selected in all twelve startup receipts. Request-read-to-grant-written
spans are 21.615/24.4685/33.844ms. These are B-local observations. Never subtract
B's timestamps or spans from A's to infer network delay, or match attempts solely
by ordinal.

The B retained worst vector with largest Dispatch has 22.884ms dispatch,
10.334ms client total/response and 9.259ms native submission nested in worker
apply. A separate B worst vector has 12.947ms dispatch, 12.291ms client authority
and 0.656ms response; the retained authority subspans are measured zero, so this
does not attribute the authority time to a particular syscall.

All 24 retained B first/worst worker-detail vectors pass presence, range,
saturation, parent-worker and nested-native checks, with one native submission
each and no saturation. First native max is 1.156ms (12 selected vectors), worst
native max is 9.259ms (12 selected vectors). Measured zero is below timer resolution.
First/worst may overlap; they are not all-input distributions. Different workload
conditions prevent a speedup or regression verdict against g180.

## Decision and validation

Keep g181's input outcome, confinement and rollback rules unchanged. Do not add
another cursor correction on the strength of this clean export. The next bounded
performance investigation should localize the source grant-response wait; native
submission and per-input authority delay remain separate candidates. A future
entry mismatch still requires its comparison flags before choosing a correction.

Validation: exact-revision isolation; unique host-local event identities;
same-host connection timelines; frame/histogram and grant/read conservation;
startup selection invariants; worker/native presence, saturation and parent bounds;
source span definitions; `git diff --check`. Skills: desktop-app and
performance-measurement-integrity-review.

Runtime tests, rebuilding and native PIN trials are skipped because this change
records inspected evidence only. No product version, source-of-truth contract,
API, database, runner, installer, security policy or active process changes.
Intermittent delay and the earlier mismatch cause remain unresolved.
