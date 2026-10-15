# g306 input preparation after a clean return

The user completed g305's continuous lock, PIN login, passkey and opposite-
direction test without a disconnect, but reported about 300ms before B's cursor
appeared on A-to-B entry. Paired collection
`a0ca540ded42cb8a0b16c9a3cc208c02` contains 11 matching handoff exchanges.
The first two B receivers used prepared children. The remaining nine reported
PreparationNoCandidate, with ReadyWait 140767–173799us. Cold child connection
was 100689–127347us and overlaps child validation; these spans are not additive.
B's capture cursor prepare/show/restore observations were all below 10ms.
The data identifies a repeated cold-start cost, not actual rendered cursor time.

Capture-owned return cursor hiding no longer opens a passive presentation lease.
The service's existing preparation-after-presentation callback therefore does
not run on that route. Movement preparation hints remain rate limited, so quick
reentries can launch a new child even while the connection stays healthy.

After one successfully recovered ordinary input lease, g306 prepares at most
one unbound child before accepting the next service request. No timer refreshes
it, no input is replayed, and the existing two-second expiry, single claim,
session/aliveness checks, authenticated bootstrap and cleanup fences remain.
The return receipt is timestamped before preparation, preserving its original
admission deadline. Presentation/capture leases, expired leases, cancellation,
and failed or unknown recovery cannot trigger this path. Construction failure
falls back to the existing cold launch; uncertain cleanup still stops the service.

This spends a bounded speculative launch after a clean return even if the user
does not reenter. It targets quick reentry only; a first entry, entry after expiry,
or immediate reentry before construction completes can retain a startup delay.
Device latency improvement and continued lock/PIN behavior require measurement
with the g306 build. No wire format, authorization, installer or UI changes.
