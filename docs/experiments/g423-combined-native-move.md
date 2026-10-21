# g423: combine native movement and cursor readback

The user's severe B-side stutter report follows g422. Paired collection
`18c1fc79f0a2c226a55853a31c4f059f`, exchange
`7c16ae06cd06b17291dfb9250c49ae6add35eab8d9ba61663c868b1764b5698f`,
contains a B receiver sample with client total 280150us, worker apply
135626us and cursor observation 114668us. These nested software spans are
not an end-to-end physical latency measurement.

The journal worker decomposed opMoveRead into native-child opMove followed
by native-child opCursorRead, despite the child already supporting opMoveRead.
Use that existing combined command only for the concrete isolatedInput owner.
Each move now needs one child exchange and one child desktop admission rather
than two. Input still executes before real cursor readback. The child retains
its fresh authority/desktop checks, sequence/binding validation and failure
certainty; no input retry or cached cursor observation is introduced. Pressed
input journaling, child retirement and independent release recovery are intact.
The parent profile records the combined exchange as one containing apply span,
without inventing separate native-stage measurements.

A regression exercises actual child request/response encoding, verifies one
exchange, desktop admission, move and readback, and preserves unknown outcome
after readback failure without replay. Existing direct-native readback/retry,
submission and isolated-worker recovery regressions also pass. Full inputworker
tests and packaging/application results are recorded below when available.

A fresh pre-change doctor reports UI RUNNING, session FAULTED and protected
service STOPPED. Source response-reader backpressure and active-heartbeat
retirement remain separate investigation candidates; this change does not
claim to fix the service's recurring 1066 or reconnect failure. Do not close
those issues based on this optimization. No public API/wire schema, DB,
dependency, runner, firewall or signing changes; no remote push.
