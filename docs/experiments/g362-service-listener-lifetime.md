# g362: Own input/source pipe instances for the service lifetime

## g361 attended failure

The operator completed the requested idle-entry, quick returns and B
lock/PIN/login sequence, but B showed an error immediately before the final
response. It subsequently reconnected. This is a failed stability trial.
Collection `0997c86cdc10fe2f68d637cd941ca18b` preserves both sides.

On 2026-10-19 B records desktop switches at 20:01:16.985795Z and
20:01:17.19243Z, a protected return-cursor connection at 20:01:17.5360201Z,
and its cancellation at 20:01:17.6563671Z. At 20:01:17.9076229Z the new input
connection fails with pipe_open / native 2. The triggering failure is
request-authorize at 20:01:17.9082045Z. A observes remote EOF; do not subtract
the PCs' timestamps to infer latency. B's service remains READY with its
19:57:33.7399883Z start observation. Agent restart/reconnection follows.
The service did not crash in this trace. Some control history is incomplete.

G361 reserved a successor only after a clean lease. A cancelled presentation
or another recoverable error still closed the endpoint before joining optional
cleanup and applying retry backoff. The same missing-name class could also
occur after accept/authentication timeout. Retaining the clean-path exception
and enlarging missing-name grace would leave endpoint lifetime inconsistent.

## Ownership change

Replace the clean-return rebind helper with one exclusive server instance per
input/source endpoint for the service lifetime. Each serial channel borrows a
non-inheritable DuplicateHandle reference to that instance. Closing the channel
no longer removes the bound name during failed-lease cleanup, optional child
preparation, accept timeout or the existing retry backoff.

Only the serial loop may borrow again. It first verifies that the preceding
channel has closed and joined all its I/O; the surrounding loop still joins
owned workers and native recovery, and exits on ErrRecovery. It then calls
DisconnectNamedPipe before lending the next handle. No client PID, permission,
lease proof or unread request data is carried into the next authentication.
Uncertain closure/reset/duplication fences the endpoint. Shutdown closes the
borrowed handle and the retained instance, preserving close errors.

The next ConnectNamedPipe remains after native cleanup and preparation. Thus a
client may observe the existing bounded busy-pipe wait, instead of a falsely
missing endpoint. Missing-name grace, connect/startup deadlines, backoff,
authorization and input outcome/replay rules are unchanged. The source endpoint
uses the same owner to avoid a sibling close/recreate gap during desktop refresh.

API semantics checked against Microsoft primary documentation:
[DisconnectNamedPipe](https://learn.microsoft.com/en-us/windows/win32/api/namedpipeapi/nf-namedpipeapi-disconnectnamedpipe)
and [DuplicateHandle](https://learn.microsoft.com/en-us/windows/win32/api/handleapi/nf-handleapi-duplicatehandle).

## Verification

Private native endpoints test a 350ms recovery/backoff interval, longer than
the unchanged 250ms input missing-name grace. Dial remains pending through
the interval and connects to the same server PID after reset. Old unread bytes
do not cross into the next connection, and a still-open retired client cannot
write to that new connection. Tests also cover a cancelled unconnected accept,
refusal to reset an outstanding owner, exclusive name ownership and handle
closure on shutdown.

Injected reset/duplication errors retain native 6 with their fixed operation
in the persisted service receipt. Unknown prior closure remains ErrRecovery.
The new labels are service-pipe-owner/reset/duplicate/close; no paths, handles,
process identifiers, input values or raw error strings enter diagnostic output.
Inputworker, logindiag, agent, peer and residentagent tests and their
`go vet -unsafeptr=false` pass; the final stale-client native regression also
passes. These tests create private kernel pipes, not product input or services.

Generation 362 is a local candidate at this checkpoint. The running pair still
uses g361, and real lock/PIN/movement acceptance remains unverified. Bugfix and
desktop-app procedures apply. No public CLI/API, DB, dependency, runner, trust
policy, reboot, service restart, force kill, push, public release or CI run.

## Verified package

Workspace build passes 85 UI tests, native compilation, protected installer
provenance, archive roundtrip and eight-file/icon checks. Offline update verify
returns valid/ok for generation 362, source revision
`1857565d81f90081407e9470dd26dc67859ff552`, archive size 27714432 bytes.
Independent PowerShell SHA-256 values:

| File | SHA-256 |
| --- | --- |
| krasis.exe | ae6a15c89d56f49c860183c939b2ea4d9c008f57c2cfb88149ad7239cdddd622 |
| krasis-agent.exe | 080b84e22236d136a7ce80e6aaeec01131cc28f37dc9e67cceb24a44825560ec |
| krasis-cli.exe | 1ff9aec92a1aa96f5b6768c88d44dd7f6d60dd31b1f894d46ea346b664727705 |
| krasis-install.exe | 79215701f8f740b1be0dd16d8f22aac3d9e3ea103b80c603330b0cf8ca8bca4f |

Generation 362 is packaged at this checkpoint; attended acceptance is pending.
