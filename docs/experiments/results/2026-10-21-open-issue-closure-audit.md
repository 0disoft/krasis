# Open issue closure audit — 2026-10-21

This is an incomplete acceptance audit, not a release or hardware approval.
The user excludes Dependency Dashboard #1 from closure and prohibits Git push,
remote ref updates and issue comments. Those constraints remain in force.

Latest correction (g424): #50 is CLOSED at 11:34:04Z. Its live original
completion criteria require separated injection/network diagnostics, bounded
cleanup and correct failure/pressed-state recovery, all covered by inspected
current code and executed regressions. The earlier extra prerequisite of the
entire physical Winlogon matrix did not belong to #50; that matrix remains
in #6/#7/#20. See 2026-10-21-issue-50-acceptance.md. Other rows below remain
an incomplete historical audit, not a current g424 device acceptance claim.

## Current authoritative observations

- Local main is clean after source eb60473 and paired-application record e3c0c30.
- Installed g416 source is eb6047375746698ecba7635239f59b53c9c75673 on both PCs.
  The paired collection and exact installation receipts are in
  `../g416-guard-preparation-budget.md`.
- Fresh doctor at approximately 03:00Z reports healthy CONNECTED, matched
  UI/resident/installed revisions, running service and completed stage cleanup.
- GitHub lists #1/#4/#5/#6/#7/#15/#16/#20/#50/#59/#70 open and no open PRs.
  Only local main exists. No remote mutation was performed in this audit.
- GitHub's commit endpoint returns HTTP 422, "No commit found for SHA", for
  the exact g416 source revision. Exact-revision hosted CI cannot be inferred.
- CurrentUser/My and LocalMachine/My have zero currently valid certificates
  with a private key and the code-signing EKU. The configured local signing
  certificate, thumbprint and timestamp environment values are all absent.
  No secret value was read or logged. Availability of an external signing
  service is unknown; absence of local configuration is not a claim that none
  exists elsewhere.
- The g415 Computer Use audit still showed the Windows Security firewall
  permission prompt. No input, permission change or screenshot persistence was
  attempted. The existing user handoff has no recorded answer.
- The user's normal round-trip/click/keyboard/drag/wheel report is for g377.
  It does not supply the additional cases or current-revision acceptance below.

## Requirement-by-requirement disposition

| Issue | Evidence present | Required next evidence; not yet achieved |
| --- | --- | --- |
| #4 | Earlier study-pair results; g377 ordinary input report; current local queue/admission tests. | Same clean CI-green revision on the identified study pair, all admission-blocking cases, schema-valid checked aggregate reports and a redacted result. |
| #5 | Runbook and checker exist. H-A/H-B remain UNDECIDED in the device ledger. | Identify the home pair, record its redacted inventory and run the applicable baseline and bidirectional probes. Current A/B labels do not establish that location. |
| #6 | Lifecycle/guard cleanup implementation and controlled fault regressions. | Attended physical termination/held-input, network, lock, sleep, topology and uncertain-injection cases, with checked bounded recovery reports. |
| #7 | Input instrumentation, controlled queue/stall tests and g377 ordinary-use report. | Actual supported device/DPI/privilege/rate combinations, bounded latency distributions and soak, then evidence-based support limits. |
| #15 | Product UI/control implementation, 85 UI tests and labeled connected accessibility tree. | Reviewable screenshot, keyboard/accessibility/localization/high-DPI and state coverage, plus publication/exact-revision CI. The security prompt currently blocks the screenshot review. |
| #16 | Full application/install/update/recovery/remove implementation and successful g416 paired upgrades. | Publisher signing configuration and signed candidate verification; full current-build device install/remove/data/firewall/rollback acceptance. A successful upgrade alone does not prove the full matrix. |
| #20 | Linked core/product implementation work and the supporting local records. | All linked acceptance issues completed, required default-branch CI, consistent four-device evidence and a signed operable/rollbackable/removable beta candidate. |
| #50 (CLOSED) | Current diagnostics, injection/cleanup budgets, stalled real-child input/release retirement, partial/unknown outcomes and exact pressed-state/LOCAL recovery regressions. | Original completion criteria verified; full physical release/Winlogon matrix stays in #6/#7/#20. See dedicated acceptance record. |
| #59 | Observer isolation, hook-progress fences, native retirement owners and g415 cursor reentry fix. | Required desktop/cursor callback behavior and actual physical progress/retirement evidence. Removing immediate desktop checks or detaching callback input is not an acceptance substitute. |
| #70 | Authenticated route-discovery implementation and endpoint identity/stale-profile regressions. | Actual two-PC address/endpoint change and authenticated reconnection, with unchanged-address DHCP and different-identity rejection kept distinct. |

## Resume conditions

The same incomplete hardware/signing/CI/UI conditions persisted through the
g414, g415 and g416 rounds. The latest rounds fixed separately reproduced local
defects; they did not satisfy or remove those acceptance conditions.

The next closure steps require attended device trial results, a usable publisher
signing setup, user handling of the Windows Security prompt, and later explicit
authorization for the currently forbidden push/publication steps. Existing
handoff questions remain pending; silence is not approval or a test result.
Do not request private keys/passwords in chat, create substitute self-signed
publisher proof, fabricate physical reports, weaken acceptance criteria, or
close unfinished issues merely to empty the tracker.

No further source defect was established during this audit. No new build or
device replacement is warranted by these unchanged acceptance gaps alone.
No implementation tests were rerun for this documentation-only record;
`git diff --check` is the relevant verification. Sources and runtime data remain
untouched. No public API/DB, dependency, runner, firewall, certificate trust,
repository hygiene, branch/worktree or remote state change is part of the audit.
