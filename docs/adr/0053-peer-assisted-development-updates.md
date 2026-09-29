# Peer-assisted development updates

Status: accepted

## Scope and authorization

The operator requested one-click transfer and application of a development build
to a paired laptop after one local opt-in. Input-control permission alone does
not grant update permission. Opt-in must bind the exact paired certificate, be
revocable locally, and be rechecked before receiving and before applying a bundle.
The existing unsigned development ZIP is not an official signed release. The
opt-in delegates code supply to that paired computer; hashes only detect damage.
No generic remote shell, arbitrary path write, or arbitrary file execution API
is part of this feature.

## Delivery stages

1. Validate and snapshot the complete distribution.
2. Transfer over a separately bounded, mutually authenticated connection so file
   traffic does not hold the input-session write lock.
3. Verify fixed file names, total and per-file size, SHA-256 checksums, platform,
   clean Go build identity, matching revision, and a higher agent generation.
4. Stage in a new user-private directory; never replace the running executables.
5. Recheck permission, drain the input connection, launch the new GUI, and verify
   the expected agent build before reconnecting. Downloaded is not installed.

The first supporting version must be installed manually on both PCs. Unknown or
old peers must not receive a new input protocol message. Retained old directories
provide a manual recovery path; automatic downgrade is not authorized.

## Increment 1

`internal/updatebundle` implements bounded ZIP validation, distribution packing,
and fresh-directory staging. It never executes received code.

## Transfer and local API contract

Generation 21 uses HTTP/1.1 over the existing pinned mutual TLS roles on the input
listener's port plus one. The listener serves an explicitly queued bundle; the
connector polls every two seconds only while authorized, or uploads its queued
bundle. TLS 1.3, certificate expiry/revocation, current CONNECTED peer identity,
and per-device opt-in are checked separately. Input protocol v1 is unchanged.
Transfers have a 90-second limit, offers expire after three minutes, and only one
bundle is admitted at a time. There is no blind retry of an ambiguous upload.
An accepted bundle must also be applied within three minutes of staging. The
receiver rejects an expired READY job at claim time even if its GUI was closed.

The authenticated loopback API adds POST `/v1/update` with schema
`krasis.peer-update/v1`: STATUS (no fields), ALLOW/DENY (exact fingerprint), SEND
(fingerprint plus local distribution directory), APPLY/APPLY_FAILED (fingerprint
plus exact ready job ID). Unknown actions and cross-action fields are rejected.
The LAN API cannot select local paths or grant itself update permission. Permission
is saved under the private product root; changed/revoked certificates do not
inherit it. Before APPLY, trust is rechecked and the job is consumed once under
the permission lock. New pairing/connections are fenced until handoff finishes.

The GUI retains the old distribution, launches the freshly revalidated bundle,
and exits normally. The new GUI waits for old GUI ownership, uses the existing
agent-upgrade handshake, checks the exact expected runtime build, and reconnects.
During APPLYING, that handshake may stop the old agent only when the new GUI
declares the exact generation and revision of the claimed update. Unrelated or
legacy launchers cannot consume the handoff.
The sender GUI also prepares its agent and reconnects after the receiver's stop.
The sender refuses a new transfer while its running resident agent is from a
different generation than the GUI. The update dialog reports both versions so
the operator can disconnect and recover the local agent first.
Before sending, an authenticated live observation of the exact current or a
newer peer build prevents a redundant transfer. An unknown send result can be
resolved to SAME_VERSION only from that exact live build; stale observations do
not prove completion or trigger an automatic resend.
Local launch failure clears the apply fence. If the new GUI starts but agent
startup fails, it reports APPLY_FAILED only for the exact active job, trusted
peer and expected build, then remains visible for diagnosis and manual recovery. A
new-process crash or parent-wait failure still requires reopening the preserved
old distribution; automatic rollback is not claimed. Permissions survive
restart; pending transfers deliberately do not.
The receiver GUI must be open or in the tray. Received files are never executed
solely by the network handler. Fixed stage codes omit paths, contents and aliases.

## Generation 24 management lifetime

Management workers now belong to the resident agent, not an input session. Saved
profiles are reconciled every five seconds (at most 16 workers); changed profiles
and revoked trust cancel old workers. Each request still checks the pinned peer,
current certificate validity, current saved address and revocation. Local update
opt-in remains mandatory for receipt and apply. LOCAL and idle FAULTED can receive
and apply; a session for another peer prevents apply. A fault latch is never reset
in place: the replacement GUI must stop the old process and verify ownership is
released before starting the new agent. The GUI must remain running or in tray.
An agent process that has crashed or cannot answer its management API still needs
local recovery; this is not an independent privileged OS service.

POST `/v2/status` exchanges bounded protocol/generation/revision/permission JSON
over pinned mutual TLS before transfer. Equal generation with a different build
is not reported as the same version. Expired or unavailable metadata never proves
that a peer is current. An unsupported endpoint and a missing response are distinct.
Generation 21-23 needs one manual upgrade to bootstrap this path. Input v1 and the
local update request schema are unchanged; response status fields are additive.
Transfer percentages indicate sender bytes consumed, not installed bytes. A
DELIVERED receipt is not installation: UPDATED requires a later authenticated
status reporting the exact expected generation and revision. Confirmation expires
after three minutes; pending job state is not persisted across sender restarts.
Shared listener-port conflicts fail closed rather than broadening certificate
trust; configure nonconflicting saved listeners for additional pairs.

Update rejection receipts use a closed `X-Krasis-Result` vocabulary. Same build,
newer receiver, equal-generation different build, denied permission, busy peer,
archive hash mismatch, interrupted transfer, invalid bundle and staging failure
are distinct outcomes. Unknown/legacy results remain unspecified rejections;
they are not inferred to mean an already-current peer. Raw error text is not sent.

## Verification boundary

Generation 61 is a transition build. Its update receiver accepts the original
seven-file archive or a fixed nine-file archive with a signed experimental
`KrasisBrokeredProbe` MSIX and public test certificate. The sender still emits
the seven-file archive so generation 60 can install generation 61. The added
files are only staged; this path does not install the MSIX or trust its
certificate. Both files must be present with bounded sizes and checksums.

Generation 75 is a second transition build. It still sends the legacy three-EXE
layout so older peers can install it, but accepts either that layout or the
three-EXE `krasis.exe` UI, `krasis-agent.exe`, `krasis-cli.exe` layout.
The two layouts are mutually exclusive; every executable remains bounded and
verified against its own Go command path, source revision, and checksum. The
receiver launches only the verified UI named by the selected layout. Upgrade
older receivers to generation 75 before sending a new-layout bundle.

Generation 76 ships the new layout from a single directory. `krasis.exe` owns
the desktop UI, `krasis-agent.exe` remains the sole resident input and trust
owner, and `krasis-cli.exe` calls the local agent API with terminal output and
exit codes. The sender and receiver use the same directory. The old layout is
still accepted so an older saved distribution can be inspected or recovered.

Generation 77 makes the sender select the adjacent g75 legacy-layout bundle
for every peer below generation 75, then the current three-EXE bundle after
that peer reports g75. This avoids sending a format that older receivers
reject. If g75 is not adjacent, SEND reports a missing transition instead of
attempting the incompatible bundle.

Generation 90 is an update-format transition. It still sends the ordinary
bundle accepted by generation 89, but its receiver also accepts six fixed,
bounded UIAccess experiment files alongside either optional brokered probe
pair. Each optional set must be complete and checksummed. Received files are
only staged; peer update never trusts the certificate or runs the experiment.
Upgrade a generation-89 receiver to generation 90 before sending a bundle
that includes the UIAccess experiment.

The attended g97 comparison bundle contains a newly signed Escape-format
UIAccess probe and its public test certificate as the same complete six-file
set. Delivery only stages the files. Certificate trust, installation, test
execution and removal remain attended on the destination PC.

Generation 93 selects the adjacent g90 distribution for peers on generations
75 through 89, then the current distribution after the peer reports g90.
This keeps the transition and experiment in the same update UI without asking
the operator to copy test files or switch the sender to g90. Each send remains
an explicit click, and a missing g90 distribution fails before transfer.
Generation 94 applies the same adjacent-g90 selection to `krasis-cli update
send`, so a terminal client cannot accidentally offer the probe bundle to an
older receiver. The CLI also fails before transfer if g90 is missing.

Unit tests cover denied and revoked permissions, single-use apply, malformed ZIPs,
generation checks and integrity checks. Packaged two-PC upgrade, firewall behavior,
native GUI relaunch and post-upgrade input require manual verification. No official
publisher signature, production feed, global rollout or release is claimed.
