# g544: preserve cursor height across the paired screens

## Observed defect

The destination entry calculation always used `top + height/2`. Natural return
sent only the lease identity and deactivated source capture without adjusting
the original source pointer. Consequently an upper/lower crossing entered the
destination at its center and returned at an old source height.

## Change

- Normalize the crossing Y into `1..65536`; reserve zero for absent information.
  Project onto the destination's current virtual screen with integer rounding.
- Preserve the existing Windows hook X/Y in one atomic snapshot. Forward height
  through the bounded boundary queue, preparation/emergency observers and lease
  request. Linux forwards the Y from its existing idle X11 position sample.
- On natural return, use the receiver's current injected pointer Y, not its entry
  Y. Disarm/rearm still match session/epoch independently of presentation data.
- Reposition the returning source through its existing authorized capture
  deactivation on the native desktop owner. Stale identities, inbound revocation,
  planned retirement, abort, shutdown and replacement desktops do not reposition.
- Negotiate `krasis/1.height` via authenticated TLS ALPN. Keep both older ALPNs,
  v3 lease messages and the geometry-free v4 preparation hint as fallbacks.
  Only request/revoke messages with a height use the v5 extension.

Each height-bearing network control payload adds exactly 4 bytes. The private
source packet stays 64 bytes. There are zero additional network round trips,
zero additional worker RPCs, and no per-movement OS position reads. Windows
return adds one local cursor observation and one position write; entry retains
its existing placement/rollback transaction. A 2160-pixel destination has less
than 0.017 pixels of height-ratio quantization before final pixel rounding.

## Verification

Unit coverage includes upper/lower bounds and adjacent pixels, center and its
neighbors, negative origins, mixed resolutions, degenerate/overflow geometry,
legacy fallbacks, malformed/truncated payloads, private packet field isolation,
the request/entry/revoke/source pipeline, stale lease rejection, and preserving
cleanup order while forwarding the negotiated capability through the evidence
wrapper. Authorized Windows entry is checked with an injected cursor port;
ordinary cursor I/O remains prohibited in that test.

Paired installation and physical cursor continuity are recorded separately from
unit/compile verification. Human crossings are needed to confirm the final
visible behavior at the top and bottom of both real notebook screens.

## Completed verification and paired installation

- Related functional tests passed for control, transport, peer, wininput,
  inputworker, agent, residentagent and localapi. New cursor-height tests also
  passed with `-race` in control, transport, peer, wininput, inputworker and agent,
  using the already cached portable Clang with CGO enabled.
- Vet passed with the existing CI policy: ordinary vet for the related packages
  and `-unsafeptr=false` for wininput's existing native callback casts. The
  unrestricted attempt reported only those existing casts; no checker policy
  or workflow was changed.
- Linux amd64 cross-build passed for the seven affected runtime packages, and
  the Linux wininput test executable compiled. Native Linux/X11 tests were not
  run: this installation targets the two Windows notebooks and the VM was not
  running. Compilation is not an execution result.
- The package builder passed 111 frontend tests, native UI build, installer
  checks, the real workspace update archive test, icon and eight-file bundle
  verification, and CLI `update verify`.
- Packaged source: `39b4683cfdc93c11d61aa27d3eb3e1fb666ba16d`, generation 544.
  Exactly one B transfer was submitted: `7c78c751718afb81fd2183117f462a28`.
  Its exact receipt reported `UPDATED`, with 29,268,953 of 29,268,953 bytes.
- A setup returned exit code 0 and `installed`. Doctor verified CLI, agent, UI,
  launch record and installed agent against that revision; service was running,
  protected input configured and the connection healthy. B status reported the
  same generation/revision, ONLINE and update IDLE. CLI `app connect B` returned
  `connected` with receipt `f4dcae0412c57892758e940b735b0f00`.
- Diagnostic collection `4d5d0c9e13fdda9a07bf11b92d02d7cb` independently reported
  input READY and login service READY on both machines. Both logs identify
  connection `6b28fa223d7ad78a4f3f8a02581cbc3f6b41188fa9309cc1e180128963e6495c`.
  B's startup acquire retries while waiting for A's installation are retained;
  A's stored failure marker predates its new service start. These historical
  entries are not represented as a clean full-history error count.

No flight recorder was enabled. No public release, push, remote CI or physical
cursor continuity claim is made by these checks. Manual top/bottom crossings in
both directions remain the final visible-behavior check.
