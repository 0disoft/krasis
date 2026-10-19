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

## Paired application

B update `ec2384640900d41d8a70256d7c9eaec7` reaches UPDATED for the exact
g361 revision and 27895265-byte archive. A opens the verified new bundle
through the existing single-instance replacement procedure. A UI, agent,
launch record and installed agent all reach g361 with matching paths and
revision. The saved B connection is started once after A reaches LOCAL.
No force kill, manual service start or reboot is used.

A doctor returns healthy / CONNECTED / PROTECTED / CONFIGURED with service
RUNNING. Peer update status observes B ONLINE at the same revision, IDLE.
Fresh paired collection `8872f2312ec9dadc05a1d6b281a32561` is RECEIVED with
both files created at `2026-10-19T19:59:23Z`. Both service/input-health states
are READY, with no lastFailure. Service start observations are A
19:58:10.00523Z and B 19:57:33.7399883Z. Automatic installation receipts are
A 19:58:08.8306445Z and B 19:57:31.452698Z, both result 0 / complete /
not_needed; actual automatic installer stdout/stderr/exit codes were not
independently captured. No remote B doctor was executed.

The new-revision logs contain CONNECTED at A 19:58:31.5456178Z and
B 19:58:32.1814964Z, with no triggering session failure retained in this
post-update window. Earlier startup acquisition retries occurred while the
pair was being replaced and the saved connection had not yet been resumed;
they are not classified as failures of the new attended movement trial.
Both control.previous.jsonl files remain incomplete.

The operator was asked for an idle first entry, six quick returns and one
B lock/PIN/login sequence. That human result and its corresponding fresh
movement evidence are pending. The goal remains open; a short clean startup
window and local regressions do not establish long-term resolution.

## Attended result

The operator completed movement and lock/PIN/login steps but B errored just
before the final response, then reconnected. Treat the trial as failed.
Collection `0997c86cdc10fe2f68d637cd941ca18b` confirms B pipe_open / native 2
after a cancelled return-cursor presentation; its service remained READY.
The remaining lifetime gap and the g362 correction are documented in
`g362-service-listener-lifetime.md`.
