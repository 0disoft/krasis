# g353 paired rollout

Source revision: `26b9760d09e0b30ebfe1c0fc5ea52c61038bfd6d`.

Includes bounded input cadence diagnostics, native proof for an optional heartbeat
that expires before kernel write submission, and one-second auxiliary control
connection reuse with fresh trust authorization on every request. Movement jitter
and the g352 trust-freshness expiry remain unresolved pending device evidence.

## Local validation

- Go tests and vet: inputworker, productlog, handoff, peer, agent, residentagent,
  localapi, peerupdate.
- Focused control-transport race tests with the existing portable compiler passed.
- Desktop-tagged workspacehost tests passed.
- Frontend: 85 tests passed; native production assets built.
- Workspace package roundtrip, installer provenance, icon/layout checks, and
  offline `update verify` passed. No new dependencies or CI runs.

Independent PowerShell SHA-256 values:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 8eaed5e6a3f8f76df82c6fef11487f5f8fbd1944198fe55c27ff9124e8f3b036 |
| krasis-agent.exe | 8711709e36852c2c4ed0b718adb906ac0e7d23b9eca8a1c4e01677c9d6f85422 |
| krasis-cli.exe | 3a8cfbe1f81a80e5a3c67508642935cce09f8193521e10c89109b31a67bdcb87 |
| krasis-install.exe | 6a65df4be23a80a9528effe4003e83dbc66f7e6e81982035f25740a327bcb702 |

## Device baseline, 2026-10-19 UTC

B update `f06c35a14d731b42800768dac26806d6` reached `UPDATED` for the exact
revision. A's normal UI replacement automatically refreshed protected input;
the existing B profile was reconnected with CLI. No reboot or manual service
start was used. A doctor returned healthy/CONNECTED/PROTECTED/CONFIGURED with
matching UI, installed agent, and running service. Peer status reported B as
g353 ONLINE/IDLE at the same revision.

Paired collection `843f80e2de00872ba9f9589aac03897e`, created at 12:24:48Z,
was RECEIVED. Both supports have installation result 0/stage complete and
service/input health READY. Their retained events for the exact g353 revision
contain no fault or failed event. Both include an incomplete older
`control.previous.jsonl`; this short baseline is not a long-term stability test.
Actual installer exit/stdout were not separately captured from automatic refresh.

Next attended trial: idle 15 seconds, enter B once, continuously move there for
about 20 seconds, then make three round trips. Collect a new immutable pair and
interpret source/receiver cadence separately. Initial entry delay and subsequent
movement jitter need separate user observations; no additional PIN trial is
required for this performance step. No push or public release was performed.

## Later paired observation, 12:33:15Z

Collection `b4d822722dd5a4544fcd93b09e27ce30` was RECEIVED. Both services and
input health remain READY; the retained g353 events contain no session fault.
B's acquire-network timeouts occurred during the earlier A replacement, before
the 12:24:48Z baseline. A doctor remains healthy/CONNECTED/PROTECTED and B's
management observation is g353 ONLINE/IDLE. These observations do not substitute
for the requested attended movement trial or a long-term stability check.

Two ordinary source/receiver profiles match by exchange tag, independently of
wall-clock offsets. In tag `6959899c...`, A records 430 ordinary dispatch samples
(maximum 6224us); B records 429 (maximum 16464us). The next tag `5a1a9280...`
has 23/22 samples, with maxima 561us/3053us. The source and receiver begin their
sample populations at different boundaries; count differences do not prove loss.

Their input-start gap histograms use <1ms/<4ms/<10ms/<25ms/<100ms/>=100ms:

| Tag | A source | B receiver |
| --- | --- | --- |
| 6959899c | 9/5/343/38/23/12 | 0/75/213/101/28/12 |
| 5a1a9280 | 0/0/21/0/1/1 | 0/6/10/4/1/1 |

Both sides have long gaps, and the receiver distribution spreads across short
and longer intervals. This is evidence to examine arrival cadence and delivery
as well as native dispatch, not proof of Wi-Fi causation. The continuous-motion
duration and intentional pauses are unreported; independent distributions and
maximum gaps cannot be subtracted or matched per input. Existing source request
to grant-read spans are 35915us and 79802us; those include both hosts' work and
transport. Receiver startup marks both workers prepared. This excludes neither
later worker cost nor other contributors to perceived stutter.

Keep g353 running for the pending attended trial. The more detailed lock-file
trust observer is separately committed locally and has not been deployed. No
input replay, wider timeout, policy change or transport replacement is justified
by these two uncontrolled observations alone.
