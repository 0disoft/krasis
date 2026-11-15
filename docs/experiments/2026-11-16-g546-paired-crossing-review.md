# g546 physical crossing review

The user completed the requested crossings and reported remaining stutter.
Fresh collection `831297fcefb3fb7a7b0358bb6f84f155` was RECEIVED. Both notebooks
remain on g546 revision `9b1aeaef4b95c11b951be0c8f82548359b5b4407`.
The JSON companion contains input hashes, matching tags, measured masks,
per-exchange details and the g545 baseline collection.

## Health

Doctor is healthy/CONNECTED; peer is ONLINE at the same build, update IDLE.
Both input/service states are READY and their service starts match installation
collection `7ab946fb4f2c22cb54a635ce8497a1c5`. One common connection has been
observed for 212.670s on A and 214.429s on B. No post-connect timeout,
reconnection, fault, poison, session failure or recovery-required event appears.
B's six acquisition timeouts and RECONNECTING marker precede this connection,
during staggered installation. Stored lastFailure values are older failures.

Eight source/receiver profiles match; all have end=ok, none are incomplete or
unmatched. All eight use prepared workers. Agent current/previous logs are
complete; control.previous is incomplete on both devices. This is a short,
bounded observation, not evidence of indefinite fault-free operation.

## g545 versus g546

Local software durations in ms; g545 n=11, g546 n=8. Trials were not controlled
for host or network load. Nested spans and independent maxima are not additive.

| Operation | g545 median | g545 max | g546 median | g546 max |
| --- | ---: | ---: | ---: | ---: |
| A boundary to entry begin | 2.216 | 5.652 | 5.050 | 29.925 |
| A request to grant | 39.552 | 61.972 | 49.016 | 325.072 |
| A capture activation | 20.527 | 47.237 | 34.924 | 60.450 |
| B readiness response wait | 25.395 | 36.337 | 24.266 | 119.843 |
| B first input dispatch | 1.880 | 2.866 | 1.813 | 2.312 |
| B per-lease worst input dispatch | 2.952 | 7.126 | 3.427 | 4.678 |
| B SendInput in worst-dispatch sample | 1.089 | 2.901 | 0.591 | 1.733 |
| A natural-return capture deactivation | 10.644 | 13.494 | 11.500 | 22.206 |

The batched decoder's lower read/allocation count is verified separately by
the g546 microbenchmark. This physical trial does not establish a material
entry-latency improvement: readiness median is similar, while tails are worse.
The different samples cannot establish that the decoder caused the regression.

## The waits are not one event

In exchange `a217e4a10a2f49d0ab2f873c3c65ceebe0eb3b8142c02e836d426d848837e52d`,
A request/grant wait is 325.072ms. Its matching B readiness is 25.216ms and
B authorization total is 30.220ms. A reader progress assigns the whole span
to ReadWaiting, with zero dispatch/between-frame time; B request-to-grant-write
is 36.302ms. A socket RTT estimate changes from 18.105 to 71.838ms.
ReadWaiting includes transport, peer work and scheduling, not pure network
latency. No opposite-PC wall-clock subtraction or RTT subtraction is used.
All eight TCP query pairs have known counters with zero retransmission/timeout
increments; that does not prove absence of network jitter.

In the separate final exchange
`12063ddac41be074ed869c90d426bc6a2d35cc74214424ef3617df32d12cc8be`,
B readiness is 119.843ms and A grant wait is 161.240ms. The candidate was
prepared and 351.882ms old. Matching startup diagnostics retain bootstrap-ready
104.375ms, worker agent authentication 28.790ms, and desktop/native-pair readiness
54.760ms. These overlap and must not be summed. Release-child service
authentication is 43.101ms in its native-startup profile. Earlier full-image
hashing belongs partly to prewarming; it is not all current-entry delay.

In exchange `e8e020fcd435a21e736eb1b3fd1c44fc37a648f5234523250a00106bb140c552`,
A capture activation reaches 60.450ms, but its matching IPC response span is
23.508ms, child activation 19.844ms, exact foreground request 12.171ms and
client native authority proof 0.554ms. Thus the largest capture total cannot
be attributed wholly to the foreground API; some elapsed time is outside those
retained detail spans. The current evidence does not identify its exact owner.

## A runtime scheduling observations

A's largest main-loop timer lateness is 3593.797ms; sampler lateness is
4336.108ms, both in the 18:54:21 UTC summary before the retained crossing
profiles, with zero input dispatches. This is not a measured 4.3s cursor freeze.
The 18:56:20 summary covering crossing activity records main timer 51.655ms,
sampler 421.302ms and dispatch max 59.001ms. B's corresponding sampler is
6.658ms. A slow OS-read observation is diagnostic, not a fault or proof of
network disconnection. Host/runtime scheduling remains an investigation target;
CPU saturation, GC and network causes are not isolated by this collection.

## Verification and next target

Verified build identity, health, unchanged service starts, common connection,
eight complete matched profiles, measured fields and baseline comparison.
No runtime source, policy, API, DB, runner or installation was changed in this
analysis; source tests were not rerun. No physical input-to-display measurement
or controlled CPU/network experiment was performed. Prioritize separating A's
grant-read/receive-resume delay from transport, then the independent B readiness
tail; do not treat a decoder microbenchmark as a fix for either tail.
