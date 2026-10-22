# g438: identify failed source-write origin

Status: built, verified, and installed on A/B. Both installation receipts are
successful, both protected-input states are READY, and local doctor is healthy
with matching g438 UI/agent/installed images and a CONNECTED peer.

g436 was withdrawn after live source-write timeouts. The resolver observed
installed_source_reader_eof; a later child report can describe a separate
shutdown write. Neither observation establishes the first failure's cause.
g437 restores g435 writer/query behavior, and this change retains that behavior.

Attach one closed local origin to each failed write: boundary, action, reply,
return, or retirement (including the stop acknowledgement). Preserve the existing
deadline, unsubmitted, partial, timeout, closed, native, and other reasons.
Append operation codes to the existing authenticated child report table and
allow only these fixed combinations in sanitized runtime diagnostics. No key,
coordinate, action payload, path, or free-form error string is exported.

Selected transport failures retain their write origin through the final report;
a later retirement failure must not overwrite an earlier failure. Native cleanup
uncertainty still takes precedence. Admission, deadlines, sequencing, stream
poisoning, authority checks, retirement proof, and recovery policy are unchanged.
This is diagnostic work; intermittent latency remains unresolved.

Validation covers all five origins through writer failure and report transport,
earlier-failure precedence, cleanup uncertainty, the closed export allowlist,
and the byte-sized operation table. The disconnected-agent retirement test now
expects the refined retirement/deadline operation while preserving its existing
checks that a clean native retirement cannot stop the service.

Passed: `go test -timeout 90s ./internal/inputworker ./internal/logindiag
./internal/productlog ./internal/residentagent -skip
'^TestPortablePairRetainsIndependentRecoveryAfterInputFailure$'` and
`git diff --check`. The portable pair fixed-owner fixture is excluded to avoid
conflicting with the installed session. No hosted CI, race detector, or
physical-input acceptance.

No public API, frame layout, DB, dependency, runner, hygiene, or product and
architecture source-of-truth change. The private report enum gains appended
diagnostic values, so paired installation must use matching g438 components.
Local main only; no push or remote ref changes.

Rollout passed 91 frontend tests, the native build, update-package smoke test,
eight-file bundle checks, and offline verification. B update reached UPDATED;
A repair installer exited zero and app repair returned recovery_ready.

Pre-rollout collection also captured an A service failure on g437, followed by
portable execution; A repair restored the protected service. This predates g438
deployment and does not establish causality for either g436 or g438.

Fresh paired observations still show A heartbeat response timeouts. The source
exit at 14:08:33Z retained reply_gate as its lifetime slow stage (at least one
second); another reader timeout followed at 14:09:48Z. All three observed A
source retirements joined their native/sender owners and sent clean retirement,
while the service stayed READY. B observed a peer heartbeat timeout and clean
retirement at 14:09:49Z; final local doctor recovered CONNECTED. No source-write
origin failure in this sample identifies the first failed write. Aggregate
client/worker maxima cannot attribute worker latency to an unanswered request.

Next: identify which writer holds the reply gate, separate reader-idle expiry
from network heartbeat timeout, and investigate g437's source-command-loop
recovery report independently. Do not weaken authority checks or native cleanup
proof, and do not claim the intermittent issue is resolved. Rollout evidence:
results/2026-10-22-g438-source-write-origin.json.
