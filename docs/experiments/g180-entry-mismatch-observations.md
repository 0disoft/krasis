# g180 entry mismatch remains observable

Revision: `587bccd22c7d7bfe59373d0a072663b9fffc90cb`.
These operator exports are observational, not a controlled performance comparison.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 17:23:30Z | `7C9764BDA6D4B0C3043955F87E2F8E5AF02BC6749E632C492CD9B61F7BD69F3E` |
| B | 2026-10-05 17:23:26Z | `950B81BC5D057604F7716434EF9B24D19DC0137C81B9D3EE16704A34A78F07D0` |

## Connection failure and subsequent successful crossings

Each host retains twelve profiles: eleven complete `ok` profiles and one failed
attempt. B's local attempt 5 ends `error` and incomplete; A's local attempt 5 ends
`incomplete`. Neither retains an input frame in that failed attempt. Local ordinals
are not independently verified wire correlation IDs.

B's same-host sequence is lease-ready, then `injection_failure` at
17:22:31.6507198Z (`entry`, `OUTCOME_UNKNOWN`, cursor stage `mismatch`, counts
unknown), then `reconnect_control_dispatch_failed` and `RECONNECTING` at
17:22:31.6638517Z. One TLS handshake failure precedes final `CONNECTED` at
17:22:34.0264323Z. The failure-to-connection interval is about 2.4 seconds on B.
A separately records remote EOF/reconnecting at 17:22:31.7840688Z and final
connection at 17:22:34.1484014Z. Do not subtract timestamps across hosts.

On each host, four successful retained profiles precede the failure and seven
follow final reconnection. No further failure is recorded to export: about 55.9
seconds on A and 52.0 on B. These seven profiles provide post-reconnection input
evidence, unlike the g179 cohort. They do not establish long-term stability or
successful PIN-open crossings, whose specific operator context is not recorded.

Earlier B acquisition timeouts (24) and a manual disconnect precede first
connection. The source of those earlier timeouts is not diagnosed here. B has 67
lease-ready events in this revision and no installed-input connection or heartbeat
failure code. Each export has one incomplete historical log with no current-build
events in it. Host-local identities are unique; histogram conservation passes:
A 1,006 [1002,4,0,0,0], B 995 [71,907,17,0,0].

## Failed entry details and source boundary

B's failed attempt has startup preparation decision 1 (`used`), prepared age
405.719ms, ReadyWait 6.485ms, EntryPrepare 5.565ms and AuthorizeTotal 27.532ms.
These are nested process-local spans and must not be summed. A's incomplete grant
receipt reports 43.423ms, zero decoded reads/bytes/frames and no complete grant.

`PrepareEntry` compares the private actual readback with the independently
computed target before sending a grant. On the installed g180 path, placement
selects a permitted target from the observed confinement and performs set/read
inside one worker operation. A mismatch is still rejected. The failure proves
the g180 change was insufficient to eliminate the observed entry problem; it
does not establish that clipping caused the g179 failure or that g180 made it worse.

The export does not identify whether confinement or geometry changed after its
initial observation, whether another input changed the cursor, or whether a
coordinate-space/native behavior differed. No target, actual position or rectangle
is retained. Source routes this entry error through rollback before protected
lease retirement. Absence of a rollback failure is not a positive restore receipt.
The prepared worker succeeded; changing its lifetime is not supported by this
failure's evidence.

## Successful input latency remains a separate target

B's slowest successful retained worst vector (local attempt 2) has Dispatch
27.226ms, ClientTotal/ClientResponse 26.689ms, measured ClientAuthority zero
microseconds, worker authority/scope/apply 3/0/8 floored milliseconds, and
apply/submission 8.861ms with a measured zero-microsecond snapshot. ClientResponse
includes worker and delivery/scheduling; it is not an independent addend.

Another vector (attempt 8) has Dispatch 14.828ms, ClientAuthority 13.637ms and
its pipe-owner subspan 13.637ms, apply/submission 0.629ms and response 1.190ms.
Authority delay and native submission are distinct candidates. Do not combine
these maxima into a synthetic input.

All 22 successful retained worker vectors satisfy presence, range, saturation,
parent-worker and nested-native bounds, with one native submission per vector and
no saturation. First native duration is min/median/max 0/0.587/1.907ms (11 vectors);
worst native duration is 0/0.675/8.861ms (11). First/worst can overlap and are selected
samples, not all inputs. Zero is below timer resolution, not absent work. Different
workloads prevent an improvement claim from comparison with g179's 17.709ms call.

## Next bounded work and validation

Prioritize classifying the remaining cursor mismatch inside the worker, without
retaining coordinates: distinguish observed constraint/geometry changes and
coordinate/readback disagreement before choosing another correction. Keep one
cursor write, independent expected-target verification, rollback ownership and
unknown/no-replay handling. Do not accept any actual cursor position as a valid
target or reset clipping to hide the failure. Native submission optimization is
separate work and does not correct this failed cursor placement.

Validation: exact-revision isolation; host-local event/timeline review; frame,
histogram, grant-duration and byte-gap conservation; startup selection invariants;
worker/native presence, saturation and parent bounds; failed and successful cohort
separation; source inspection; `git diff --check`. Skills: desktop-app and
performance-measurement-integrity-review.

Runtime tests, rebuilding and new native PIN/rollback trials are skipped because
only this evidence document changes. No product version, API, database, protocol,
installer, runner or security-policy changes. Cursor-entry mismatch and intermittent
delay remain unresolved.
