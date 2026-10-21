# g409: bounded source reader and watchdog authorization (#50/#59)

The read-side command and presence watchdog used direct service/agent native
queries. A controlled delayed-native regression reproduced a 50ms caller taking
two seconds and returning success. Both owners now receive one shared one-second
budget (an earlier worker cancellation/deadline wins), with independent full and
presence lanes under the existing process-wide query capacity. No result is
cached; presence still cannot authorize commands requiring a full proof.

The capture bootstrap transfers the service proof and control pipe together to
a Channel owner after startup cancellation is stopped. Shutdown interrupts the
control reader by deadline without closing a query's pipe. The agent pipe also
transfers to its Channel instead of retaining an unconditional bootstrap Close.
Expired native queries keep handles/capacity until actual return; pending service
cleanup is reported as uncertain rather than successful. Capture/sender/native
thread joins and the protected hook's desktop-affine checks remain intact.

Verification: the old direct-call implementation failed the controlled deadline
test. Focused tests cover stalled service/agent full and presence queries, 50ms
expiry, exactly-once handle retirement, and one shared budget across both owners.
The existing query capacity, late-success, heartbeat independence, and native
resource-retirement tests are retained. Broader package checks/application status
are recorded below after execution. Race detection is unavailable with CGO=0.

This is supporting implementation evidence for #50/#59, not completion of their
native callback/physical acceptance gates. User-confirmed round trips, click,
keyboard, drag and both scroll directions belong to g377; no newer-build physical
acceptance is inferred. Dashboard #1 remains open. No public API/DB, dependency,
runner, signing, firewall or remote Git ref change.
