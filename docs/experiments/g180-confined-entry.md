# g180 confined cursor entry

## Evidence and correction

g179 recorded a B `entry/mismatch` failure and reconnection after its twelve
successful retained profiles. The export does not establish whether confinement,
physical movement or a desktop/geometry change caused that particular mismatch.

Source inspection found that protected entry set the requested cursor position
and read it back in separate worker operations, comparing it with the original
request. Windows may adjust positions to existing cursor confinement. This build
corrects that supported case without accepting arbitrary mismatches.

The native worker reads current confinement, computes the permitted target within
screen bounds, performs one cursor set and reads back in one authorized operation.
The agent checks actual readback against that independently computed target.
The protected lease wrapper forwards this combined operation to its selected
client; it does not emulate it with separate set/read requests or use ordinary
cursor APIs. Snapshot-before-entry and rollback ownership remain intact.

Limits: confinement or physical cursor movement can still change during the
operation. Actual mismatch remains a failure. A lost ACK cannot be retried, and
an unknown placement remains pending for existing recovery. Ordinary input
backends that lack placement retain their previous entry path. This change is
not a claim that every g179 mismatch or native `SendInput` delay is resolved.

## Validation and device follow-up

Hosted regressions cover negative and exclusive rectangle bounds, disjoint or
invalid confinement, clipped placement, actual mismatch, cancellation before/after
mutation, native query failure, one cursor write, single worker scope/ACK,
truncated private suffix, following-ACK alignment, pressed-state recovery, missing
capability, lost ACK without replay, selected-client forwarding and adapter rollback.
Native API calls are stubbed; no live cursor or PIN input is injected by these tests.

Go desktop tests and vet (`-unsafeptr=false`) pass for the ten related packages:
agent, transport, peer, residentagent, handoff, inputworker, productlog, localapi,
wininput and application. The focused backend slice also passed inputworker/wininput
tests and vet before its independent commit. Formatting and diff checks pass.

Apply g180 to both devices and their protected input workers. Test ordinary
crossings, then keep B's PIN dialog open while returning to A and reentering B.
Export both diagnostics after the trial. A missing failure in a short export does
not prove the confinement branch executed or permanent stability. Native device
trials and actual performance improvement remain unverified here.

Source-of-truth contract updated: ADR 0064, confined cursor placement section.
Local worker operation/ACK extension only; no external API, database, runner,
installer policy, ACL, UAC, certificate or clipping-policy changes. Procedure
selection: bugfix and desktop-app.
