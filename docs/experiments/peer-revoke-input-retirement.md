# Retire the installed inbound owner after a source-initiated revoke

## Evidence and reproduction

The g360 B support collection `e2e043ea7fb2cfdfc82797cf0d4bc337` records
`installed_input_admission_blocked_active` immediately before its
17:47:38.8601246Z request-authorize failure on 2026-10-19. This identifies
an existing native owner, without identifying how that owner survived.

Source inspection finds that peer REVOKE routes through
`OutboundController.RevokePeer` even for an inbound lease. Runtime revocation
returns to LOCAL and releases held input, but did not disarm the inbound
return lifecycle. Installed `privilegedBoundary.DisarmReturn` owns native
lease/heartbeat closure, so the next lease could see a non-nil client.
The management-only revoke path had the same omission.

`TestRunSessionPeerRevokeRetiresInboundOwnerBeforeNextRequest` reproduces
the defect with the actual `agent.RunSession` wiring and a framed connection:
REQUEST, GRANT, COMMIT, REVOKE, then REQUEST ends in EOF on unchanged g360.
The fixture models the installed authorizer's one-owner requirement and uses
no native input injection. After the fix, three requests share one connection.
Two held-key releases complete before their matching input owners retire.
This reproduces a causal implementation defect; it does not prove that this
was the exact initiating action in the historical attended session.

## Change and verification

The agent wires the responder's return owner into peer revocation. Successful
inbound revocation disarms the matching return intent before releasing the
shared coordinator. Outbound revocation and rejected stale revocation do not
disarm an inbound owner. No revoke is echoed. Receiver handoff retirement now
uses the receiver profile instead of attempting to retire a source profile.
Management revocation uses the same ordering and requires its return owner
at construction.

Agent, peer and residentagent package tests and their
`go vet -unsafeptr=false` pass. Focused tests cover the framed reentry case,
key-up-before-close, exact inbound retirement, outbound/stale isolation and
management revocation. The framed regression failed before implementation
with `second lease grant: EOF` and passes after implementation.

This is a local source fix; the running g360 pair is unchanged. Real lock/PIN,
long-running movement and a new packaged rollout remain unverified. No trust
policy, input replay, deadline, public CLI/API, DB, dependency or runner change;
no push, release or CI execution. Bugfix and desktop-app procedures apply.
