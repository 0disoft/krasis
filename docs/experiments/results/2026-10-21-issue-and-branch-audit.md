# Issue and branch audit — 2026-10-21

## Repository state

- Audited local `main`: `6cdfdb4` (g368 application record).
- Remote `main`: `a41cf88df061162365aef5b97bbf809fe100f67d`.
- No open pull requests. No push, release, new generation or device replacement in this audit.
- Ten non-main local branches and ten non-main remote branches were deleted after history review and bundle verification. Only `main` remains on both sides, with one checkout.
- Merged feature branches were already represented by squash merges on main. No additional implementation needed importing. The old issue-19 documentation hash tests were not restored because they would regress the current contracts.
- Local recovery archive: `.git/krasis-improvement/issue-audit-20261021/branches-before-cleanup.bundle`. This archive and the branch inventory are local-only and are not part of the commit.

## Issue disposition

Closing an implementation issue requires its remaining acceptance conditions to be met. Local tests do not supply physical-device, external-policy or signing evidence.

| Issue | Decision | Evidence or remaining work |
| --- | --- | --- |
| #1 | Closed as not planned | `renovate.json` disables the dependency dashboard. Retiring the dashboard does not complete its pending dependency updates. |
| #4 | Open | Same-build study-pair input, admission, disposition and aggregate-report acceptance remains incomplete. |
| #5 | Open | Home-pair inventory and baseline reports require access to those physical devices. |
| #6 | Open | Crash, held-input, network, lock, suspend and topology fault matrix requires observed two-device outcomes. |
| #7 | Open | Hardware polling, DPI, privilege, input compatibility and bounded soak measurements remain incomplete. |
| #15 | Open | Tray/settings and actual keyboard, accessibility and high-DPI acceptance must be checked against the current UI. Publication is deferred by the maintainer. |
| #16 | Open | Authenticode, full-app install/remove/data cleanup and interrupted-install acceptance remain incomplete. |
| #20 | Open | Umbrella tracker depends on the hardware, GUI and signed-beta conditions above. |
| #49 | Open | Ordered-stream pause and stalled-receiver tests pass. Actual TCP retransmission/zero-window attribution and device acceptance remain incomplete. |
| #50 | Open | `0f152d7` and `ea5141d` bound cooperative injection/admission and preserve cleanup ownership. A native call ignoring context remains synchronously owned until it returns; see ADR 0008. Do not treat that case as fixed. |
| #57 | Open | Synthetic 125/1000/4000/8000-event bursts coalesce to bounded queue depth, preserve discrete/generation boundaries and pass failure tests. Hardware rates and two-device held-input/soak evidence remain incomplete. |
| #59 | Open | Hook timing exists. Raw-versus-hook progress mismatch detection and safe owner replacement remain unfinished. |
| #67 | Open | Power/network observation and unknown-versus-observed classification need implementation and validation; a heartbeat timeout alone does not identify wireless power saving. |
| #68 | Open | Roaming observation and a single bounded reacquisition after restoration remain unfinished. |
| #70 | Open | Authenticated address recovery exists; actual endpoint-change acceptance remains pending. See `docs/desktop/address-recovery.md`. |
| #71 | Open | Selected-route observation and bounded reevaluation must distinguish local route changes from peer endpoint changes. |
| #72 | Open | Windows policy evidence and transport error classification are incomplete. Timeout alone must not be labeled a firewall block. |
| #86 | Open | Input-path mismatch classification and safe recovery need further work. No external security product has been identified as the cause. |
| #90 | Open | Trust-read contention mitigations and timing exist; repeated contention location and sustained two-device/held-input acceptance remain pending. |

## Verification

Passed on the audited checkout, using repository-local Go caches and `GOPROXY=off`:

```text
go test ./internal/application ./internal/peer -run 'Test(StalledInjection|SlowKeyUp|ReleaseAllStopsExpired|InputProgress|ProgressSeparates|SessionRuntime.*(Deadline|Admission)|.*Runtime.*(Deadline|Admission))' -timeout 90s
go test ./internal/wininput ./internal/peer -run 'Test(SourceSession|OrderedStreamPauseKeepsHeartbeatBehindIncompleteInput|PausedReceiverDoesNotValidateQueuedHeartbeat)' -timeout 90s
```

The first command passed during the preceding audit segment and was reused without changes to its inputs. The second passed in this segment. Tests use controlled adapters/streams; they are not physical polling-rate or TCP packet-loss measurements.

No source, dependency, API, DB, runner or product-contract changes were made. Full release checks, device disruption, signing and hosted CI were not run because this segment only records the audit. Existing g368 deployment evidence remains separate from this audit.

## Next priorities

Ranked by user impact, expected benefit and implementation/verification cost:

1. **#90 and recurring recovery:** collect fresh paired diagnostics from the installed g368 before selecting another fix. Highest direct user impact; collection is inexpensive and can distinguish trust failures from the newer source-retirement failures. Do not attribute a new failure to the historical trust issue without evidence.
2. **#59 hook liveness:** design bounded raw/hook mismatch detection that ignores idle periods, then retire the current owner before replacement. High impact for silent input loss; medium cost, with synthetic and native validation needed.
3. **#50 remaining native stall:** assess the worker/guard lifetime boundary for calls that cannot honor cancellation. High impact but higher implementation cost; retain uncertain pressed state and avoid replaying input on another backend.
4. **#57 and #49 device acceptance:** reuse current coalescing and delay tests, then collect bounded two-PC queue/latency/recovery evidence when the user can operate both devices. No new queue-size change is justified by this audit.

No new implementation task was started after the audit's ten-minute cutoff. The remaining issues stay open with explicit outstanding conditions; they must not be closed solely to clear the issue list.
