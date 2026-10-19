# g354: Input TCP acknowledgement cadence experiment

The g353 attended trial completed, but the user reports frequent stutter during
movement. Paired collection `8dbdd9ede481b84b886f8373efc57b1f` is RECEIVED; both
services and input-health records are READY. Its earlier session also contains a
B heartbeat timeout and A remote EOF. READY at collection does not erase that
failure. Do not search only top-level `fault_*` codes: session faults are fields
of `session_end_detail`, with trigger versus teardown distinguished.

## Retained movement evidence

The long matched tag `49a68bdacd8feaf63aa3f4eebe27dcbd24c650e8d09a3ee65c351b3849e6c37e`
has source cadence 1645 intervals, distributed 10/30/1436/134/26/9 across
<1ms/<4ms/<10ms/<25ms/<100ms/>=100ms. Receiver cadence has 1639 intervals,
distributed 3/1136/153/220/106/21. B's ordinary dispatch maximum is 11786us;
A's is 62299us, including coordinator admission. Both sparse and compressed
arrival periods exist. Source queue maximum residence is independently 726329us
in its retained runtime interval. These maxima are not additive and the
different sample populations do not establish lost input or per-frame latency.

Sampled source TCP estimates range well above their minimum RTT. Those are
kernel estimates near grant acquisition, not movement-packet latency. Earlier
heartbeat failure trails show no GC pause in their final retained samples;
they do not exclude GC elsewhere. A preserved execution trace has only about
25.5ms aggregate runnable delay across its captured interval and long passive
native waits. It is not aligned evidence of the later continuous-movement trial.

## Bounded change

After successful peer TLS authentication, request `SIO_TCP_SET_ACK_FREQUENCY`
with `TCP_ACK_FREQUENCY_PARAMETERS.TcpDelayedAckFrequency = 1` on that connected
Windows TCP socket. The parameter is one byte. This removes one possible local
ACK batching delay without changing the wire protocol or retransmitting input.
This is an experiment prompted by cadence variation, not a proven diagnosis of
delayed ACK, Wi-Fi, the adapter, source admission or scheduler behavior.

No registry, adapter setting, process priority, DSCP, QoS, firewall, global TCP
policy, auxiliary-transfer transport, lease bound, security check or two-second
peer timeout is changed. There is one synchronous socket request per authenticated
connection, no per-input syscall, new packet payload, timer or retained handle.
ACK traffic may increase. Any rejected/unsupported request preserves the normal
connection and reports unavailable; it does not retry or fabricate success.

`tcp_ack_every_packet_requested` means only that Windows accepted the request,
not that packets were delivered, a worker acknowledged input, peer liveness was
renewed, or performance improved. The unavailable/not-TCP/unsupported alternatives
are also fixed sanitized codes. The policy disappears with the connection.

Native loopback TLS tests on A verify that the actual IOCTL succeeds for both
authenticated endpoints. Failure classification and caller-owned unsupported
stream usability are tested separately. Physical effect on movement and B's
native acceptance remain unverified until deployment and an attended trial.

Local transport, peer, agent, residentagent, productlog and localapi tests and
vet pass. Focused transport/productlog race tests pass. This validates socket
setup, existing delivery/failure behavior and redaction, not a latency reduction.

Primary API definitions:

- [Microsoft IOCTL constant](https://microsoft.github.io/windows-docs-rs/doc/windows/Win32/Networking/WinSock/constant.SIO_TCP_SET_ACK_FREQUENCY.html)
- [Microsoft one-byte parameter](https://microsoft.github.io/windows-docs-rs/doc/windows/Win32/Networking/WinSock/struct.TCP_ACK_FREQUENCY_PARAMETERS.html)

The bundle also carries the separately committed detailed trust lock-file
observer. That addition supplies attribution only and does not cache trust.

## Paired rollout

Bundle revision `47772998cbfb2416127cae54e47840f246e8a57c`, generation 354,
was built with the ordinary workspace script. All 85 frontend tests, the native
build, desktop workspacehost tests, installer provenance, package roundtrip and
offline bundle verification passed. No push, public release or CI run was made.

Independent PowerShell SHA-256 values:

| File | SHA-256 |
| --- | --- |
| krasis.exe | d59a4933ccd5098213f79720494188dbd18f198a9186224378c3fe1257c8f873 |
| krasis-agent.exe | a5e873bb88039602f8eec4a7f3ab4789145c14de214d19779bcb9713a69149c9 |
| krasis-cli.exe | 14e9bbcd07c3e1e3c4b48acb8b0be7ffa830ff7239eaf1adbc0e75156a78caea |
| krasis-install.exe | 5a777a53c00f53d1e63946b1651c7103b2ba3fc0e77a11e4f142d8568d2384a1 |

B transfer `787c5cb842058f2c3f3c0780030e4796` reached UPDATED for that revision.
A then used normal UI replacement and automatic protected-input refresh, followed
by reconnecting its existing B profile. No reboot or manual service start was used.
A doctor reports healthy/CONNECTED/PROTECTED/CONFIGURED with matching UI and
installed agent; B is g354 ONLINE/IDLE at the exact revision.

Collection `37ec7868faad2d6e8561cd18a92f9cd7`, created 2026-10-19 12:53:21 UTC,
was RECEIVED. Both installation receipts have result 0/stage complete and both
service/input-health records are READY. Both actual product connections emit
`tcp_ack_every_packet_requested`; no session termination trigger is retained for
the new revision in this short baseline. Both supports retain an incomplete old
control log. Automatic installer process exit/stdout were not captured separately.

The next attended comparison remains 15 seconds idle, A-to-B entry, about 20
seconds continuous movement on B and three round trips. Source/receiver cadence,
local dispatch costs and perceived jitter must be assessed separately. Acceptance
of the option is confirmed on both PCs; its real movement benefit remains pending.

## Attended movement result, 12:59:52Z

The user completed the requested idle/continuous-movement/round-trip trial and
reports that g354 clearly stutters less than g353. Complete elimination of stutter
was not reported. Retain g354 as the current device build; this observation does
not call for another deployment or another immediate user trial.

Paired collection `d6f1eeda5ee5f4e711bcd36cf58bef53` contains A support created
at 12:59:52Z and B at 12:59:53Z. Filter events by exact bundle revision
`47772998cbfb2416127cae54e47840f246e8a57c`. Both supports retain installation
result 0/stage complete/recovery not_needed, service READY and input-health READY.
Both actual connections record the ACK option request being accepted. There are
no retained session termination events, nested session-end triggers or top-level
fault/failed events for this revision. B's acquire-network timeouts end at
12:53:04Z, before the established baseline and movement trial. They belong to
rollout reconnection, not the later movement observation. Both supports still
flag the older control.previous.jsonl as incomplete. This is a bounded successful
observation, not long-term stability evidence.

All 13 source profiles have a corresponding receiver profile with the same
exchange tag and end `ok`. The longest g354 movement tag is
`31b76b659ccf8efe74372bab7967b6d28a3fdf7d30f1b2c16ff50d26263bbdda`.
Compare it descriptively with the longest retained g353 tag above:

| Build / local role | Intervals | <1ms | <4ms | <10ms | <25ms | <100ms | >=100ms | >=25ms share |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| g353 A source | 1645 | 10 | 30 | 1436 | 134 | 26 | 9 | 2.13% |
| g353 B receiver | 1639 | 3 | 1136 | 153 | 220 | 106 | 21 | 7.75% |
| g354 A source | 1620 | 0 | 4 | 1600 | 10 | 4 | 2 | 0.37% |
| g354 B receiver | 1615 | 0 | 305 | 1004 | 279 | 24 | 3 | 1.67% |

The receiver now has fewer long gaps and fewer compressed <4ms intervals. This
agrees with the user's improvement report, but source cadence also changed.
These are separate trial populations with intentional pauses and different
sampling boundaries, not a controlled causal estimate of ACK policy benefit.
Do not subtract host timestamps, match histogram entries as packets, infer lost
input from count differences or interpret cadence as input-to-display latency.

## Remaining local tails and next investigation

For the long g354 tag, ordinary source dispatch maximum is 2186us and receiver
dispatch maximum is 78778us. The receiver's selected worst operation retains
ClientTotal 77434us, ClientAuthority 4552us and ClientResponse 72882us. Its ACK
read waits 72882us until the first positive Read, then records zero floored
microseconds for header/suffix decode and reaches matched progress. The matching
worker reports a 27613us native submission call. These overlapping measurements
must not be added or subtracted to manufacture a missing-stage duration. They
point to the receiver's worker execution/response observation path; they do not
attribute all of its wait to the pipe, scheduler or worker alone.

That same entry has source request-to-grant 232392us, receiver request-read to
grant-write 225271us, and receiver startup marked not prepared. Retained local
startup spans include service child connect 154729us and worker installed
validation 67817us (load 67232us within validation). These are a separate entry
preparation cost from subsequent movement. Source capture activation is 9375us.
Respect each span's nesting; do not sum overlapping service/worker work.

Earlier g354 profiles also retain receiver ordinary dispatch 103852us, so the
long movement tag is not the worst operation across the collection. Runtime
summary maxima additionally include non-movement dispatch work. A's maximum
retained source queue residence is 7669us. The next bounded code investigation
should inspect receiver preparation reuse and worker response/native submission
wait ownership before proposing another transport change. Preserve authority,
input order, matched ACK requirements and existing timeout bounds.

Validation for this result is read-only parsing of both immutable supports,
exact-revision fault/session-end inspection, exchange-tag matching and histogram
sum/percentage checks against the handoff schema. No product code, API, DB,
runner or policy changed; no new build, restart, push, public release, CI run or
additional PIN test was performed. Code tests are not rerun for this evidence-only
documentation change.
