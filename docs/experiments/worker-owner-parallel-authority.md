# Parallel worker-owner authority

## Physical baseline, 2026-10-28

g478 (`f7cf3ed6`), collection `128eca1dd5b2c744c8a458d439908c4f`, has 12
complete successful matched A-source/B-installed-receiver leases in the physical
test window. Both services are READY and no recovery failure appears in that
window. A capture activation median/max is 11.329/14.203ms; B receiver-ready
median/max is 33.4835/403.434ms. The slowest ready profile did not use a prepared
worker. Its bootstrap-validation and native-pair-ready spans are
195.745ms and 180.854ms, respectively. These spans overlap other stages; they
cannot be summed or attributed wholly to owner authentication.

One separately paired 255.149ms request/grant exchange spends 242.408ms inside
the receiver. Matched residual time is not pure wire RTT. Observed TCP
retransmitted bytes and timeout episodes are zero. Native SendInput max is
3.549ms. This points investigation toward receiver readiness, but does not
establish a single explanation for every stall.

## Defect and change

`authorizeWorkerOwners` checked the agent proof and then the service proof.
These independent read-only checks have separate pinned owners. Serial execution
requires A+B time and can exhaust the shared deadline even when each check
would finish within it. Input startup and passive cursor presentation both use
this gate. Run both queries concurrently under the existing common deadline and
join both bounded callers before admitting execution. The all-success condition,
fresh queries, process-wide native capacity, timeout fencing, and late-query
handle retention remain mandatory. No proof cache or recovery bypass is added.

## Controlled verification

A fake-clock regression uses 30ms and 40ms queries with a 50ms owner budget.
Before the change it rejects at 50ms; after the change it succeeds at 40ms.
The unconstrained critical path changes from 70ms to 40ms (42.9% less query
waiting), not a claim about physical handoff latency. Native I/O count stays two.
The bounded caller adds one goroutine and one single-result channel per gate.
Either owner's denial is rejected only after both callers join. Existing tests
cover caller/lease/default deadlines, query capacity, canceled admission, fresh
proofs, fencing, independent final reporting, and retained handles.

g479 must be measured on both laptops before claiming a physical latency gain.
Rollback uses a new generation containing the serial gate; never overwrite a
running generation or reuse an old executable/metadata pair.
