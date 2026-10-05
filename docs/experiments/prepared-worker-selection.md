# Prepared worker selection diagnostics

g174 has one 113ms receiver readiness wait without the prepared-worker marker,
while eleven marked profiles wait 6..9ms. A missing marker does not identify why
the service used a fresh child. g175 adds a service-owned selection diagnostic
before changing expiry or preparation scheduling.

`StartupProfile` schema 1 keeps its existing eight timing stages and optional
prepared age. Optional `preparationDecision` is a fixed numeric code:

| Code | Meaning |
| --- | --- |
| 0 or absent | Decision unobserved; includes legacy receipts |
| 1 | Prepared candidate used |
| 2 | No candidate at lease admission |
| 3 | Candidate expired |
| 4 | Candidate process exited |
| 5 | Console session invalid or changed |
| 6 | Atomic claim already taken or candidate explicitly retired |
| 7 | Claim context cancelled or unavailable |
| 8 | Candidate lacks a required timer/owned handle/control pipe/PID |
| 9 | Candidate age is invalid (negative) |

Only code 1 agrees with `prepared:true`. An observed decision requires both
service launch and child-connect spans. Unknown codes and contradictory
prepared/age values are dropped as invalid diagnostics. No code authorizes a
lease or substitutes for service, image, desktop or session authentication.

KRD5 is a fixed 64-byte local readiness envelope: the existing base/load/
prepared blocks followed by an eight-byte decision block. Its first byte is
the decision; seven reserved bytes must be zero. Missing load/prepared detail
uses zero presence and zero values. Older KRD1..KRD4 receipts remain readable,
with decision unobserved. Truncation or malformed reserved bytes fails protocol;
a structurally complete receipt with invalid numeric diagnostics is consumed
and dropped without misaligning the next frame or changing valid readiness.
The service and installed agent must match through existing image checks before
using KRD5. No peer frame or per-input ACK changes.

The service assigns the selection reason before a cold fallback and publishes it
only after independently verified worker readiness. Child diagnostic merging
cannot overwrite this service decision. Failure before readiness may have no
selection receipt; absence must remain unobserved, not a successful selection.
Only bounded codes, existing durations and prepared age enter the twelve-attempt/
120-second handoff export. No identities, paths, timestamps or input values are
added. No timer, request, preparation lifetime or authority rule is added.

The reason identifies the admission decision, not necessarily the preceding
history. In particular, no candidate does not prove no preparation hint was
sent, and a successful preparation receipt does not prove a reusable child was
still alive at claim. Native device effects and reason distributions require
a fresh g175 export; no latency improvement is claimed by this diagnostic.
