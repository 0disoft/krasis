# g529 parallel safety audit

## Scope and evidence

The starting checkout was clean at `141a380d` on `main`. Five GPT 6.1 Terra
agents at high reasoning reviewed separate native-input, transport/security,
application/UI/CLI, update/packaging, and diagnostics/lifecycle slices. The
coordinator reviewed the resulting diffs and the resident-agent shutdown path.
Only concrete defects were changed; findings were not treated as proof of the
cause of every previously observed latency spike or reconnect.

Source boundaries: `docs/product/02-spec.md`, ADR 0066, `VALIDATION.md`, and the
repository security/desktop/CLI checklists. Windows is the supported product
platform. Authentication, lease fences, fail-closed recovery, private IPC frame
formats, peer wire formats and the trust freshness deadline remain in force.

## Confirmed findings and repairs

| Area | Defect | Repair and regression evidence |
| --- | --- | --- |
| Input desktop | A native activity-query failure was treated as an inactive desktop. | Reopen only for the exact inactive result; preserve other failures. Joined-error regression rejects a false rebind. |
| Input worker recovery | The native release failure was discarded. | Retain the cause through `Unwrap` and fixed public error text. Tests require both `errors.Is` visibility and no private payload in `Error()`. |
| Input service | Channel-close failure could be hidden by a rejected request. | Independently enforce the cleanup fence; uncertain close requires recovery. Tests combine protocol/authority rejection with close failure. |
| Diagnostic capture | Failed quit-message delivery could leave cancellation waiting forever. | Reuse the event-backed message loop, preserving hook cleanup errors. A real Windows loop test verifies cancellation after readiness. |
| Lifecycle owner | A monitor's final published event could lose a race against its Done signal. | Preserve queued events and disallow transition recovery after confirmed monitor exit. Repeated simultaneous-ready and stopped-monitor tests. |
| Network observer | Close canceled the monitor but not its pending OS-query wait. | Give the sampler the monitor-owned context; reject canceled results. Blocked-query cancellation test. The underlying native call still has its existing single-owner fence. |
| Peer probe | An authenticated idle peer could outlive the configured timeout. | Close the socket when the probe context ends. Real TLS tests cover listener and connector reads. |
| Credential loader | A path was reopened after private-key validation. | Validate and read the same file handle in a verified directory. Replace the path after opening in the regression test. |
| Identity/trust directories | Path checks did not authenticate the newly opened directory handle on every mutation path. | Validate the exact handle used for child I/O. Unsafe inherited ACL and valid protected-directory tests. |
| Ed25519 identity | Comparing the embedded public-key suffix did not detect a corrupt private seed. | Re-derive the private key from its seed and compare all key material. A changed seed with unchanged public suffix must fail. |
| Trust observation | A shared read could recreate a missing lock file. | Missing shared-read lock is an error; regression verifies no file is created. |
| Store transactions | Active lock files could be deleted/replaced, splitting transaction ownership across two files. | Lock handles neither request nor share DELETE. Windows tests cover shared/exclusive locks, creation, multiple handles, delete/rename/replace and competing writers. |
| Unsupported-platform storage | Path-based fallback could not enforce no-reparse or atomic no-replace guarantees. | Unsupported securefs operations return an explicit error. Windows behavior remains supported; non-Windows execution is not claimed. |
| CLI | Human output failure and short writes could return success. | Checked writer returns the internal-error exit status for JSON and human output. Four output-mode/failure combinations. |
| Updated-app reconnect | An accepted request could be replayed when status still showed LOCAL. | Keep the accepted-request fence and observe without resubmission; retain uncertainty after the observation deadline. |
| Workspace shutdown | Late callbacks could add work after shutdown's Wait completed. | Serialize dynamic job admission with shutdown; reject late jobs and await admitted jobs. |
| Layout display | Alias reuse could display a different fingerprint's connection profile. | Compare the returned profile fingerprint with the selected identity; mismatches produce invalid_peer. |
| Release retention | Corrupt/unreadable ready receipts were ignored before replacing history. | Fail before staging on read/validation failures; enforce the 64-image bound before writing. |
| Probe-kit packaging | An existing ZIP's verifier was executed before its contents were trusted, with unbounded extraction. | Fixed entry allowlist, reference sizes, bounded extraction and prepared checksums; never execute archive code. Valid, tampered-verifier and oversized-entry tests. |
| Agent shutdown | The local API could close before native input cleanup completed its agent-presence queries. | Retire operations before canceling API serving and closing credentials. Preserve serve/retire/close errors, including joined cancellation failures. Tests cover successful and failed retirement and server failure. |
| Native lifecycle monitor | Failed window and thread stop messages could leave Close waiting indefinitely. | Independently check the owned stop signal within the existing 50ms message wait; retire the exact owner thread/window/class/hook. Regression suppresses both stop messages. |

Changes are committed by defect/domain. Native error details are available to
classification code without adding raw error text to public worker receipts.
The initial full test run caught a privacy regression in error-cause retention;
that regression was corrected rather than weakening the existing privacy test.

## Integration validation

The latest source passed the repository-wide `go test -p 2 -timeout 3m ./...`:
56 packages with tests passed, and eight packages reported no test files.
Repository-wide vet passed, using the existing `-unsafeptr=false` policy only
for wininput. Desktop-tag workspacehost and product entry-point tests passed.
Probe-kit valid/tampered/oversized archive checks also passed.

Ten complete package suites passed race detection: inputdesktop, inputworker,
productpanel, netobserve, agentlifecycle, securefs, peercredentials,
productidentity, truststore and peerprobe. Selected wininput, residentagent and
winlifecycle cancellation/reader/retirement tests also passed with race detection.
Desktop-tag workspacehost shutdown and UI-replacement race tests passed too.
The full wininput race suite stopped in the pre-existing direct Go invocation
of a Win32 hook callback (`source_low_level_callbacks_windows_test.go`) with a
checkptr pointer-arithmetic failure; it is not reported as a passing full suite.
No blanket unsafe-pointer suppression or test skip was added to production.

Build, setup and installed-device results are recorded in the paired
installation receipt. The first full run overlapped the final lock
implementation and therefore included incomplete-snapshot build failures; it
is not counted as a passing integration run. That run also observed a native
lifecycle resource-count assertion failure; three isolated repeats passed.
This observation must be distinguished from proven per-owner resource leakage.

The pre-install paired collection request is recorded in the final installation
receipt with its actual request identifier and support-bundle hashes.
Both protected services and input-health states were READY; A was CONNECTING,
not a verified connected session. Historical B session failures are preserved
in the support bundle and are not reclassified as current-build successes.

## Limits

Review covered production entry points and critical ownership/security paths
across the listed slices, plus whole-repository automated checks. It was not a
line-by-line proof for every source file. Some laboratory, bootstrap/login,
Windows UI and packaging paths remain without complete manual review.
Successful startup/connection does not prove attended crossing latency or
long-duration absence of reconnection/recovery failures.

The installed g529 observation retained the same authenticated connection and
READY services for approximately 99 seconds. A nevertheless reported two
source heartbeat failures, with source-service-query-cancelled and
hook-callback-stalled exits. All native owners joined and retirement completed
with clean_sent. The scheduling/native-query cause remains unresolved; these
events are preserved in the installation receipt rather than counted as a
failure-free trial.

Lock-sharing upgrades should stop old product processes before starting the
new bundle. An old writer holding DELETE access can make a new lock open fail
safely until the old handle closes. No lock failure authorizes an unlocked
transaction. No public API route, DB, dependency or CI runner was added.
Remote CI and public release publication require separate user instructions;
local installation and CLI reconnection are authorized by the current session.
