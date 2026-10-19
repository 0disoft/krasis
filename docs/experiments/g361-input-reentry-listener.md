# g361: Reserve the next input endpoint before optional child preparation

## Evidence

The immutable g360 B collection `e2e043ea7fb2cfdfc82797cf0d4bc337` records a
clean receiver handoff at 2026-10-19T17:52:16.462217Z, followed by a new
attempt whose pipe-open profile has 16 missing-name results, no busy result,
and 251771 microseconds of wait. It ends at 17:52:16.8447897Z with
`installed_input_stage_pipe_open / installed_input_native_2`. This isolates
the open failure from truststore reads or post-send input uncertainty.

The service closes the preceding worker's pipe during completed lease cleanup,
then starts and joins optional reentry child preparation before returning to
accept. Thus it can leave no named endpoint for longer than the client's
existing 250ms missing-name grace. The original trace lacks service-side
construction timestamps, so the historical preparation duration is unknown.

## Change

On a clean lease result, explicitly confirm channel closure and reserve the
exclusive next pipe instance before launching/joining optional preparation.
The loop transfers that same instance to the next accept; it does not read
requests, authenticate a peer or transfer any authority before preparation
has joined. Existing image, token, session and actual kernel PID checks remain.
The normal 250ms missing-name grace, connect/startup deadlines and input ACK
contracts are unchanged. No input or lease request is replayed.

The loop owns the reserved listener until accept takes it, including cleanup
on cancellation or worker/preparation failure. Unknown prior channel cleanup
or failure to own the exclusive successor fences further admission rather
than starting optional work with uncertain ownership. Presentation preparation
already in flight is still joined before returning a failure.

Native tests create private test endpoints and confirm that the successor
remains immediately openable while optional construction is blocked, retains
the exact server PID, and later connects on the same reserved instance.
They also reject uncertain previous closure and an already-owned pipe name.
No test calls SendInput, starts a product service or elevates a process.

Inputworker tests and `go vet -unsafeptr=false` pass. The separate inbound
revocation fix and its red/green framed regression are recorded in
`peer-revoke-input-retirement.md`. Generation 361 includes both changes;
packaging and device validation are pending at this checkpoint. The attended
g360 report is generally smooth, and g360 remains running on both PCs.

Bugfix and desktop-app procedures apply. No public CLI/API, DB, dependency,
runner or trust-policy change. No push, public release, CI run, reboot or
manual service start. Real long-term behavior remains unverified.

## Packaged candidate

The normal workspace builder passes 85 UI tests, native UI compilation,
installer provenance, archive roundtrip and eight-file/icon verification.
Offline `update verify` returns valid/ok for generation 361, revision
`6bd0dedb087a0579b13b39151b04bdbb2bee6cb3`, archive size 27895265 bytes.
Independent PowerShell SHA-256 values are:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 76f1ceb68bc37c0549791f528c8bf62de6de01f4507884f792a315418ad654b9 |
| krasis-agent.exe | f8f69df169688e6e60679384465327ee822e0fc143ec007bcb9a1876e8c93892 |
| krasis-cli.exe | 2e9678faaae700bdc16187c3d5786d433be64e09361cab75897600029a7d5377 |
| krasis-install.exe | 31c668b59779a08900ad6115dc722fb80c61abdf3b463e8184987996143b4e7f |

Packaging does not prove attended movement or long-term service stability.
