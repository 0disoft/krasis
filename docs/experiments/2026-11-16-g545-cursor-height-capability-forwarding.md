# g545: preserve negotiated height through product connection wrappers

The user reported that top/bottom crossings remained misplaced after g544.
Both devices were verified on g544 revision
`39b4683cfdc93c11d61aa27d3eb3e1fb666ba16d`, healthy and connected, with input READY.
Diagnostic collection: `bf154f6074d4905a646f889d2e96e0a7`.

## Reproduced cause

The actual connection stack is TLS PeerConnection → product disconnectConnection
→ supervisedConnection → sessionConnectionEvidence → coordinated frame writer.
g544 forwarded SupportsCursorHeight only in the evidence wrapper. The first two
wrappers hid the optional method, so cursorHeightForWriter stripped every height
and deliberately encoded a legacy v3 command. The peer consequently entered at
center or restored its old source position.

Before the fix, the new regressions failed:

- `TestSupervisedSessionTransmitsNegotiatedHeight`: enabled=true, got=0, want=1.
  This runs RunSession with the real supervisor wrapper and reads the resulting
  frame from the other end of the stream, rather than invoking a payload helper.
- `TestProductDisconnectWrapperPreservesCursorHeight`: got=false, want=true.

## Change and scope

Forward the optional method in those two existing wrappers, matching their
preparation-capability forwarding. Preserve negotiated false and the absence of
the method on older peers. No new protocol, rounding rule, I/O, round trip,
permission, lifecycle policy or retry is introduced. Generation advances to 545
for the corrected paired build. Unrelated artifact-retention scripts are not part
of this change.

The stream regression checks both enabled/disabled capability states and the
top/bottom anchors. Existing g544 projection and request/return tests continue to
cover coordinate placement; physical crossings remain a separate manual check.

## Source verification

- Functional tests passed for agent, residentagent, peer and localapi.
- Both new regressions passed with the race detector; the session-stream test
  also enables the existing preparation/emergency wrapper path and permits its
  single geometry-free hint to precede the lease request.
- Vet passed for those four packages. Linux amd64 cross-build passed for agent
  and residentagent. Native Linux UI tests were not rerun for this platform-neutral
  method-forwarding fix; compile verification is not a native test result.
