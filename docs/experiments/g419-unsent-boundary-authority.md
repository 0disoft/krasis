# g419: discard unsent advisory batches after slow authorization

The g418 paired installation is READY, but its protected source still retired
and later failed startup. The available receipts do not establish one native
cause. Inspection found a separate reproducible recovery trigger: a full native
query for advisory pointer samples uses the maintenance lane, but unlike an
unsent heartbeat its one-second timeout fenced the whole channel. The boundary
sender then canceled the capture owner although no bytes or sequence escaped.

The regression stalls that full query across its deadline. Before this fix,
the batch returns source transport failure, an independent presence check fails,
and subsequent full authorization never runs. After the fix the expired batch
is discarded without a write, heartbeat presence and an actual reply still
work, and the next full query independently accepts or denies authority.

Only the unsent advisory lane receives this policy. Pending native work keeps
its process-wide slot and proof/pipe handles until it actually returns. No
query is abandoned, replaced, replayed or cached. Close still fences and waits
for confirmed ownership; input, capture commands, revocation, failed presence,
owner cancellation, attempted/partial writes, trust freshness and retirement
requirements retain their existing failure behavior. Wire/API schemas and all
timeouts are unchanged. Advisory samples could already drop at queue overflow
and before native write submission.

This controlled reproduction does not prove it caused the recorded g418
startup failure. Remaining source-startup/guard-exit faults and physical
acceptance issues remain open. Build, tests and paired application results are
recorded below when available. No push, remote-ref, issue-comment, runner,
dependency, firewall, signing or DB change.

The full inputworker suite passes, including fresh-authority, bounded native
query ownership, rejected input, boundary sequencing and retirement cases.
Logindiag passes. In the initial wider run, residentagent's loopback TLS renewal
test exceeded its handshake deadline and localapi's exact-process cancellation
test returned a request failure. Both failed tests pass when rerun sequentially
with `-p 1`, unchanged. The first wider command therefore failed; the isolated
rerun does not establish why those timing/transport failures occurred.
