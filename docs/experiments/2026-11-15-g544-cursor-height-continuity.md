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
