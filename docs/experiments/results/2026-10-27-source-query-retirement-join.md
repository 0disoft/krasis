# Source query retirement after the g468 crossing test

The g468 collection `8c1728bab123d4178b72f598c9beec0b` contains an A activation
response timeout, a `source-agent-query-capacity` service failure and error 1066.
Capture, sender and interrupt owners joined. B remains READY. See
`2026-10-27-g468-physical-acceptance.json` for build-filtered counts and timings.
The capacity code does not identify which query held capacity or establish
whether the pending native owners eventually returned. The following race is
locally reproduced, not a proven complete explanation of that field failure.

## Reproduced unnecessary service stop

Source teardown previously inspected the three authority-query lanes once,
immediately after joining command, watchdog, hook and sender callers. One
still-running read-only native query immediately added `ErrRecovery`, even
when it subsequently returned normally. That classification survives into the
independently authenticated service report and requires administrator repair.

The regression test starts a real owned presence query, expires its command
after 20ms, then releases the native query 50ms into retirement. Before the
change the retirement function returns immediately with `ErrRecovery` and the
service-report round trip demands repair. After the change it waits 50ms for
actual retirement, preserves the rejected command, and reports confirmed cleanup.
Expired authority remains fenced and cannot authorize another command.

## Change and bounds

After all query producers join, wait on the exact existing query completion
channels. One independent `RecoveryTimeout` budget (2 seconds) covers both
authenticated owners and all three lanes. Do not create another query or
replacement goroutine, extend command authority, cache a successful proof, or
close handles under a running native call. The final post-delivery check is a
snapshot, so it cannot renew the cleanup budget.

An owner still pending after the shared budget, an unknown owner, a failed
native handle close, or a pre-existing native/sender cleanup failure continues
to require recovery. A completed canceled query's deferred handle-close error
is explicitly checked before declaring retirement confirmed. The normal path
with no pending queries performs no blocking wait; an indefinitely stuck query
can add at most two seconds at this teardown stage. Earlier hook/sender joins
and report delivery keep their existing separate bounds.

This reduces false service-stop classification, not the duration of an active
capture operation. The observed successful A activation outlier (1.37 seconds)
and failed activation (2.87 seconds) remain unresolved. The exact field query
capacity holder still needs attribution.

## Validation and application

- PASS: new delayed-query regression fails on the previous implementation and
  passes with actual query/handle retirement and service-report round trip.
- PASS: two native owners share one 2-second budget; the unjoined owner retains
  its handles and recovery fence, then closes exactly once after actual return.
- PASS: a late native handle-close failure still requires service recovery.
- PASS: `go test ./internal/inputworker -count=1 -timeout=120s` (24.434s), followed
  by focused query/report/cleanup tests after adding the handle-close case.
- PASS: `go vet ./internal/inputworker` and `git diff --check`.
- SKIP: race detector because this shell has `CGO_ENABLED=0`; the compiler/CGO
  environment was not reconfigured for this bounded change.
- SKIP: UI/build/package tests because only source teardown logic changed and
  this change has not been packaged or installed. Both laptops still run g468.
- SKIP: physical acceptance and long soak for this correction; another g468
  crossing cannot validate code that has not yet been applied.

After the user's A repair, fresh collection `363e70cf6ccf1721e8678c897736575a`
confirms both input health and service states READY. A's doctor still showed
CONNECTING; a normal disconnect/connect was requested separately. Recovery is
not acceptance of the new code or proof that connection latency is repaired.

Public API, peer/worker wire formats, DB, dependencies, CI/runners, repository
hygiene and product/architecture source-of-truth documents are unchanged. No
official release or signing operation is part of this correction.
