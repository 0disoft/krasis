# g438: identify failed source-write origin

Status: implementation prepared; packaging and paired installation pending.
The installed A/B baseline remains g437.

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

Next run: finish packaging with scripts/build-workspace.ps1, verify the package,
apply g438 to B then A using the existing update/repair procedure, and confirm
both installed receipts, protected-input READY, connection, and doctor results.
Then collect fresh paired diagnostics before choosing another behavior change.
Packaging and rollout are deferred to respect this run's 20-minute budget.
