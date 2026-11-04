# g527: Avoid forced paint and partition source request delivery

The confirmed g526 trial contains 14 complete A-source/B-receiver pairs without
new recovery or reconnect faults. A capture maximum is 114.197ms. Its request
delivery span is 40.572ms, activation 64.183ms and foreground request 23.705ms.
The largest foreground request across the trial is 55.822ms. Nearby A CPU trails
are frequently >=95% busy; this correlates with stalls but cannot establish an
individual API's kernel or external-application cause.

The request-delivery clock starts before deadline setup and the native pipe write
lock. Calling all 40.572ms pipe transit would therefore be incorrect. g527 records
an optional app-local submission checkpoint immediately before the final deadline
check and WriteFile, plus app write return. The existing worker receipt clock
partitions delivery into app admission/submission wait and subsequent transport,
worker reader scheduling and decode. Write completion is independent: the worker
may decode before the writer returns. These timings do not isolate kernel time.
No packet, byte, ACK roundtrip or authority field is added; only fixed nested JSON
diagnostics change. Unsupported streams and invalid clocks omit this detail.
Unsubmitted expiry proof, attempted-write failure and no-replay rules remain.

Window inspection finds an unconditional UpdateWindow between ShowWindow and
the foreground request. The source popup has no application-specific WM_PAINT
handler; its nonzero layered alpha, input registration and fresh foreground proof
establish ownership, rather than synchronous repaint completion. Remove that one
forced paint dispatch per activation and let the existing owner loop paint. Keep
ShowWindow, alpha/hit testing, foreground verification, fallback, rollback and
cursor hiding unchanged. CaptureUpdate stays reserved and unobserved, not zero
duration. This does not remove SetForegroundWindow or prove its 55.822ms tail fixed.

Importantly, g526's actual UpdateWindow buckets are 12 below 1ms and two below
10ms, not a dominant >=10ms bottleneck. Removal is a bounded optimization of
unnecessary work. Window activation and request-submission attribution still
need a new attended trial before claiming an overall latency improvement.

Tests cover clock zero/reversal/overflow, receipt before/after writer completion,
the exact 120-second diagnostic limit, nested/outer consistency, real Windows
activation-frame identity, expired pre-submission with no worker bytes, generic
stream fallback and no retry for partial/attempted timeout writes. Existing focus,
rollback, cursor, capture and shutdown cases remain required. Rollback is the
previous g526 installer. No DB, dependency, CI runner, peer or private IPC wire
change; no remote push or CI requested.

Validation: six related package suites passed (wininput, handoff, inputworker,
residentagent, productlog, localapi); the newly added fallback/clock/native cases
also passed a focused rerun. Standard vet passed for the five non-wininput
packages; wininput passed its existing -unsafeptr=false policy. Selected source
activation, window/focus, native submission, completion and heartbeat race tests
passed. No live g527 latency comparison is claimed by these unit checks.

Native references: [UpdateWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-updatewindow)
confirms synchronous WM_PAINT dispatch bypassing the application queue;
[layered-window features](https://learn.microsoft.com/en-us/windows/win32/winmsg/window-features)
describes nonzero-alpha hit testing and ordinary redirected painting.
