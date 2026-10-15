# g314 source standby liveness

g313 failed after the earlier five-minute observation. Paired request
`1d441cdcf7f683d675175a54169bf8ee` preserves A's source-hook failure at
2026-10-15T18:06:20.679Z. The service remained READY and the source child reported
`reader:timeout`, joined capture/sender/interrupt and clean retirement. This
incident has no trust-expiry verdict. Automatic app recovery subsequently
restarted the agent and reconnected, so current CONNECTED does not erase it.

Source reads previously applied the same two-second idle deadline even while
no input was suppressed. Keep startup, active capture, pending cursor
presentation and incomplete-frame reads at two seconds. A fully started passive
source now waits up to ten seconds for its next complete command. Once any
fragment arrives, assembly is capped at two seconds without extending an earlier
active deadline. Authenticated stop, cancellation and the absolute lease expiry
still interrupt that exact read.

Waiting passively must not delay crash cleanup: an independent 250ms presence
watch checks the same pinned process/pipe owners. Presence loss cancels the
source run, interrupts its pending read and joins the watcher before releasing
handles. The watch grants no capture/input authority. All fresh command,
desktop, input and recovery checks remain. Client heartbeat accounting uses the
same passive/active distinction; a pending cursor keeps the stricter bound.

Tests reproduce a 2.1-second idle scheduling gap: passive capture survives,
active capture expires. Partial frames still expire at two seconds; process
loss is observed in the 250ms watch; startup and pending cursor get no passive
grace. This is a state-specific liveness policy, not evidence that OS scheduling
stalls or all future failures have been eliminated.

No peer wire, API, database, dependency, ACL or runner change. Ordinary input
and lock/PIN device trials remain necessary after applying g314.

Local inputworker/source race tests passed. The first combined package run hit
the existing residentagent one-sided-renewal initial TLS deadline; a subsequent
serialized inputworker/residentagent run and vet passed without test changes.
