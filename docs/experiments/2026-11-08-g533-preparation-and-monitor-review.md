# g533 connection preparation and foreground monitor scheduling

## Evidence from the g532 paired crossing

The source is `2026-11-08-g532-paired-crossing-review.json`, collection
`b8cf8e17d7968ceaf181d59726867afa`. Twelve complete matching attempts ended OK.
No new recovery/reconnect event was found in the retained postconnection logs.
Older retained logs are incomplete; this is not an unlimited stability claim.

| Local span | Observed duration | Interpretation |
| --- | ---: | --- |
| B first ready wait | 267.456ms | First entry raced an unfinished preparation |
| B builder join in that attempt | 149.213ms | Prepared candidate age was only 5.470ms |
| B desktop readiness in that attempt | 105.228ms | Native readiness still needs actual completion |
| A slowest capture activation | 123.413ms | Worker activation accounted for 98.908ms |
| A foreground request/exchange in that attempt | 35.305ms | Includes monitor scheduling and native execution |
| A owner queue-to-handler in that attempt | 12.710ms | Exact measured queue residence |
| A activation IPC request / reply delivery | 0.779 / 0.084ms | Not a comparable-size network delivery delay |

These are nested local spans, not independent durations to add. Different
notebooks' wall clocks are never subtracted. Position/show retain timing buckets;
the evidence does not identify a specific Win32 call as the whole A outlier.

## Adopted changes

1. `f10e4d72`: the negotiated, authenticated preparation receiver offers one
   initial demand at connection startup. It retains the fresh inbound policy,
   LOCAL-state gate, 750ms timeout, two-second cooldown, cancellation and original
   15-minute candidate expiry. There is no periodic idle renewal or lease grant.
   This moves possible construction ahead of the first physical approach.
2. `2ff01c32`: the existing window-free source monitor receives the same bounded
   ABOVE_NORMAL hint as the three native input owners. It previously remained at
   normal priority despite being a synchronous foreground-activation dependency.
   The shared restoration scope preserves explicit priorities and native errors;
   a thread whose priority cannot be restored stays locked. No thread is added.

Fresh authority, desktop binding, ACK-before-input, foreground ownership checks,
bounded fallback and uncertain-retirement fences remain unchanged. Scheduling
inversion is a structural risk demonstrated by the code; its exact contribution
to the 123.413ms sample has not been separately measured.

## Validation and limits

- Uncached tests and vet passed for agent, residentagent, peer, transport,
  wininput, inputworker and handoff. Vet retains the existing `-unsafeptr=false`.
- Uncached wininput and agent race/checkptr passed. Added tests exercise initial
  preparation, the 2s minus 1ns / exact 2s boundary, no periodic demand, timeout,
  cancellation, an authenticated negotiated session and native priority cleanup.
- Actual native smoke first rejected a foreground request when the previous
  application's bounded response probe expired. The log is retained. A second
  run passed five actual activation/ownership/retirement cycles, with activation
  32.7224..90.113ms and legitimate API-rejection fallback. No bypass was added.
- Frontend: 111 tests and native build passed. Update integrity/integration,
  single-copy eight-file package and application setup verification passed.
- Full repository, non-Windows tests, remote CI and push were not run. Relevant
  Windows packages and the normal packaging workflow cover this bounded change.
- The installation receipt is `2026-11-08-g533-paired-installation-review.json`.
  Installation/readiness checks are separate from a new paired physical trial.
  Neither old maximum is claimed eliminated without that trial.

## Paired installation

Both notebooks run g533 revision `2ff01c325d501743f18d9d4b72ba874013376a83`.
CLI, agent, UI and protected installation agree. Both service/input states are
READY with PRESENT grants on the same authenticated connection. The first
36 seconds of retained postconnection logs contain no failure or reconnect.
B acknowledged preparation immediately after connection, before later hints.

Two direct setup-executable attempts on A reported failure at UI exit without
exposing the original cause. After confirming the old UI had exited, a diagnostic
helper invoked the same `appinstall.InstallWithShutdown` and
`launchrecord.StartAfterInstall` APIs with the exact verified g533 payload and
installer; installation succeeded with all original fences. No force termination,
manual pointer publication or bypass was used. The earlier UI failure is not
claimed diagnosed. Its receipts are retained in the installation report.

B had already resumed its saved connector while A remained LOCAL. CLI `connect
B` started A's listener, after which CLI `app connect B` returned `connected`.
Preconnection discovery timeouts and busy receipts are recorded separately.
