# g356: Wake and join preparation's exact pending read

g354's long movement handoff had no prepared candidate after a nearby preparation
miss. Code inspection finds avoidable work inside preparation receipt completion:
watchPreparationPeer sets a read deadline and joins its disconnected-peer reader,
but native pipe reads observe changed deadlines on a 20ms wait/poll cycle. This
can consume part of the unchanged 300ms agent preparation budget even after
unbound construction has already finished. It does not prove why that particular
preparation missed or account for all cold startup time.

The native watcher now gets its own read context. On successful or failed
construction, first disarm peer-failure cancellation, interrupt its deadline,
cancel this reader and join its return before writing any receipt. The parent
construction context is not canceled by this intentional stop. Peer EOF/extra
bytes still cancel a build that is in progress, and build/cleanup errors remain
errors. Generic connections retain the existing deadline fallback.

Pending native I/O with a cancelable context registers an exact-OVERLAPPED
CancelIoEx callback after submission. The I/O completion event wakes immediately
when cancellation completes; it need not wait for the next polling timeout.
The callback is stopped or joined before the OVERLAPPED/buffer is unpinned or
its event is closed. Pipe close still joins active I/O. Cancellation that loses
to a completed kernel operation preserves the real transferred bytes and result.
Only an actually aborted operation returns the context cancellation. Ordinary
background-context input read/write operations add no callback. Existing source
command context reads also gain prompt exact-read cancellation while preserving
their final duplex writer and next-reader usability.

The preparation budget, process lifetime, authorization, request and ACK framing,
lease expiry, input ordering and unknown-outcome rules do not change. There is
no input replay, whole-pipe cancellation, timer extension or authority cache.

Focused tests verify explicit read-context use, watcher join before return and
preservation of construction failure. A real Windows named-pipe test verifies
that watcher cancellation leaves receipt writing and a later read usable. Existing
native tests retain completed bytes across late cancellation/deadline observation,
deadline recovery, source-read cancellation and concurrent close behavior. The
full inputworker package tests and vet with only the known Win32 unsafe-pointer
analyzer disabled pass. Race instrumentation was not run: no C compiler is
available on this command environment's PATH. No UI or peer protocol changed.
Localapi tests also pass after reserving source generation 356.

This was a separate local change while the g355 attended movement comparison
was pending; at the initial commit generation 356 had not been built or applied.
Keep the
running g355 baseline until its result and fresh paired diagnostics are assessed.
No new installer, reboot, service restart, push, public release or CI run occurred
for this change. Device startup and movement benefit remain unverified.

## Prepared bundle, not applied

Generation 356 is now packaged at source revision
`db25de6f7ecd1305fe8f4a85d608061f9c10e9a1`, which includes `7d634ea` but not
the rejected event-reuse candidate. The ordinary workspace build passed all 85
frontend tests, native UI build, installer provenance, package roundtrip and
eight-file bundle checks. Offline update verification returns valid/ok, generation
356, that exact revision and archive size 27747344 bytes. Agent, residentagent and
inputdesktop tests additionally pass against this source. No live installer or
new app has been run for this generation.

Independent PowerShell SHA-256:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 50373d6411c641141c184facd2fa5619e0259f2f5a2c27b7143d162d082db9f8 |
| krasis-agent.exe | 92b6562ac0f18fef3f9f8fab84df4e2312a7424b174d98044eb84585e1063262 |
| krasis-cli.exe | d1a1dcdfaec506c521b85a17bdb15a542ff6689267bb41760d22479bbf4c7ee5 |
| krasis-install.exe | 1cece25d088ee45683a8390d4a91a2ef4c8a1b101b2c55d242a835dcfebe9aff |

Both devices still run g355. The pending user comparison is needed to separate
g355 movement effects from a subsequent g356 startup change. No push, public
release or CI run occurred. Authority and user-visible API contracts are unchanged.
