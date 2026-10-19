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
