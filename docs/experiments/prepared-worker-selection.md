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

## g175 native selection wiring

Claim preserves the existing checks and atomic transition from unbound to
claimed. Timer expiration now uses a distinct retired marker in the same atomic
state, allowing a later claim to report expiry without inspecting disposed
handles. Both expired and explicitly retired markers remain unusable; the
service's existing cleanup/replacement branch recognizes both. The timer's
compare-and-swap still cannot dispose a claimed worker. Explicit retirement,
cleanup error retention and one-child ownership remain unchanged.

For a successful cold fallback, its fresh startup timings carry the original
candidate rejection code. Timings from the rejected candidate are not substituted
for the newly launched child. A prepared success carries code 1 and its existing
bounded age. Native startup authentication and subsequent lease/desktop checks
run independently of every diagnostic code.

Hosted checks cover selection condition precedence, cancellation without claim,
missing resources, repeated claim, expired/retired distinction, expiry after
claim, retained cleanup failure, fixed receipt bounds, legacy framing, child
non-overwrite and sanitized exports. No actual input or privileged service is
started by these checks. After applying g175 to both devices, repeat ordinary
crossings and PIN-open B-A-B reentry, then export fresh diagnostics. The next
decision should use the observed rejection code and local readiness spans;
until then, preparation scheduling and the two-second lifetime remain intact.
