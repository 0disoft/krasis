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

## Bundle and paired application

Source commit is `548df37eccadef1b08e7bca469c206697ee9e684`. The workspace
builder passes 85 UI tests, native frontend/Windows executable and embedded
installer builds, provenance, archive smoke and eight-file verification.
Offline verification accepts g419, 28,295,685 bytes. No hosted CI was run.

A's old g418 protected service was STOPPED with SERVICE_RECOVERY_REQUIRED
(1066), and ordinary UI replacement failed. The existing safe-stop command and
byte-verified UI shutdown completed; no process was force-terminated. The g419
installer's attended `--repair` exited 0, with successful installation receipt
09:54:48.9648376Z. A subsequent verified UI restart launches the protected g419
agent. No Windows Security dialog was automated.

B transfer `e177d9275b6ffa201a097fe44f5091ad` reaches UPDATED; fresh inspection
reports ONLINE g419 at the same exact revision. The initial SEND response timed
out while PREPARING, but exact-ID wait confirms the original job; no duplicate
SEND was issued. B's successful installation receipt is 09:56:09.8191966Z.
Collection `51f5f5801dec1a186b0fa0da450bdce9` includes an UPDATE_REQUIRED
health sample during replacement; the later collection
`49c8537bf5e81c4f6295c1fb7837e304` confirms both PCs' inputHealth READY.
After reconnect, A's doctor reports CONNECTED, PROTECTED, all four g419 builds
matching, service RUNNING and no next actions. This is bounded current health,
not proof that every earlier failure path is repaired.

## User trial and next priority

The user reports delay when crossing into B. Collection
`bd7dd944e83e41bebe2034c868f9a447` at 09:59:34Z contains nine completed source
profiles and nine matching installed-receiver profiles, all ending `ok`; neither
current-build event history contains a fault/session failure. Counts are 725
source and 593 receiver observations, not a packet-loss comparison.

Matching by exchangeTag shows B's ReadyWait 234.1–436.1ms and AuthorizeTotal
237.9–439.9ms. A's request-to-grant is 251.0–809.2ms, with capture activation
21.1–261.0ms. These overlapping/nested software spans must not be summed as
physical end-to-end latency. B's first eight attempts use a prepared child;
their bootstrap-to-ready still takes 225.0–276.0ms. The ninth preparation
decision is 3, with ReadyWait 436.1ms. Fast queueing alone does not remove this
entry preparation delay.

Next inspect B's prepared-child bootstrap/authentication path before changing
timeouts or adding more diagnostics. Preserve installation/image/pipe/process
proof and fresh input authority. The A trace saved at 09:58:38Z (2,356,345 bytes)
is copied locally to `.cache/analysis/g419-entry-lag/runtime-stall.trace` for
offline analysis; it has not yet been analyzed and is not committed/exported.
No new implementation was started after the ten-minute cutoff.

The native trial does not demonstrate that an advisory query actually expired,
or that long-run recovery, physical fault/Winlogon cases, signing and endpoint
change acceptance pass. No issue is closed from these bounded observations.
GitHub #59's current completion wording still requires removal of callback
waiting causes and correct progress/idle behavior; this advisory-worker fix
alone does not prove that entire callback path. No open PR or non-main local
branch was found. Main remains local-only. This record changes no source-of-truth
specification, API/DB, runner, dependency, firewall or signing configuration.
