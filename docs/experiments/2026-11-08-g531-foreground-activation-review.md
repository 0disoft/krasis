# g531 foreground activation follow-up

The g530 paired report retained a 225.112ms A capture activation with 170.544ms
inside the first SetForegroundWindow request. Matching local request/reply
delivery took 0.047/0.058ms. This identifies a local native/activation interval,
not a 170ms peer network transfer. That interval still includes Windows work,
reentrant window procedures and execution scheduling; the entire historical
170ms cannot be attributed to a single internal Windows operation.

Windows processes activation synchronously when the requesting thread owns the
target input queue, and asynchronously across separate input queues:
[Microsoft's explanation](https://devblogs.microsoft.com/oldnewthing/20161118-00/?p=94745/).
The capture owner previously requested its own foreground activation directly.
g531 reuses the existing bound source monitor for the first request and keeps
the command caller's two-second timeout independent. The capture owner services
sent activation messages and verifies foreground after the request. It does not
dispatch posted input or command messages while inside activation. Fresh desktop,
authorization, expiry, terminal, foreground and ACK-before-input checks remain.
Fallback and restoration still execute on their original owner thread.

One native event per accepted activation wakes the capture owner immediately.
Two references keep it alive until both capture and the requester retire; a late
native return cannot use an already-closed handle. No requester goroutine,
process, persistent native thread, extra peer exchange or retry loop is added.
Cancellation does not claim to interrupt Win32; uncertain native retirement
continues to fence the worker. Existing foreground-request timing includes the
entire exchange and desktop validation, so moved waiting remains visible.

## Local verification

- wininput, inputworker and handoff tests passed, including cancellation before
  admission, native rejection, unverified ownership, expired/terminal requests,
  sent-message progress, native wait failure, late completion and event lifetime.
- Full uncached wininput race passed with checkptr enabled. Relevant vet passed
  with the existing wininput `-unsafeptr=false` policy.
- An initial concurrent test run failed the unchanged
  `TestServiceIPCSeparatesHandlerFromAppReadResumption`: a Sleep(12ms) interval
  measured 11,622 QPC microseconds. The failure log is preserved locally. The
  final sequential package run passed; no unrelated test or threshold changed.
- The opt-in `TestCaptureForegroundNativeExchange` passed using the real
  SourceHook, native window, input threads and source monitor, with five
  alternating original-owner/new-monitor requests. It verifies actual foreground
  ownership, deactivation and native retirement. Hosted CI skips this attended
  native test by default.

| Duration | Original owner median / max | Monitor exchange median / max |
| --- | ---: | ---: |
| Foreground request, including exchange | 12.221 / 14.780ms | 0.553 / 8.951ms |
| Complete capture activation | 78.6199 / 145.441ms | 62.1868 / 111.8951ms |

These five local comparisons are not controlled CPU-load benchmarks or paired
notebook crossing trials. Earlier exploratory smoke runs had widely varying
overall activation durations; one stopped on an existing foreground rejection
and responsiveness-probe timeout. Do not infer a guaranteed overall speedup or
freedom from reconnect/recovery events. A fresh g531 paired crossing sample is
required to compare the production 170.544ms tail.

Rollback uses the retained verified g530 setup/update bundle and normal service
update/retirement flow. Do not overwrite protected files or reuse unjoined native
owners. The architecture runtime-flow document was updated. No public API, DB,
dependency, peer/private IPC format, repository hygiene or CI runner changed.
Packaging and paired installation are authorized; push, remote CI and public
release are outside this follow-up.
