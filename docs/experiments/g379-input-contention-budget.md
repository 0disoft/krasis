# g379: bound heartbeat and protected input admission behind stalled injection

For #50, heartbeat sender/receiver code still used the unconditional runtime
snapshot lock, and ObserveHeartbeat/CheckLiveness ignored their context while
waiting for the runtime owner. A native injection holding that owner could
therefore retain heartbeat serialization after the caller's deadline. Guarded
press/release admission had the same unconditional mutex wait.

Heartbeat snapshots now use a cancellation-aware admission budget, and liveness
checks/observations honor their supplied deadline before acquiring ownership.
Protected press/release use the existing syncgate mutex. Rejected admission is
classified NOT_APPLIED; it neither prepares a new guard obligation nor clears
an old one. Fresh cleanup remains possible after the original owner retires.
Already-observed heartbeat timeout cleanup keeps its independent cancellation
budget, including cancellation racing the observation clock.

Application and residentagent suites passed. The peer suite initially caught
a timeout-cleanup cancellation regression in the first implementation; keeping
the independent cleanup budget corrected it and the full peer suite then passed.
New tests verify rejected heartbeat admission cannot refresh liveness or mutate
lease state, rejected guarded requests cannot touch native/guard state, and an
actually stalled adapter ignoring cancellation cannot retain heartbeat write
serialization or emit an unverified heartbeat. The native test owner is joined.

This does not interrupt a native operation that has already entered the OS.
That operation retains its pressed-state and native-resource ownership until it
returns. No detached injection, replay on another backend, guessed LOCAL state,
or fabricated heartbeat receipt was introduced. #50 remains open for that
separate native lifetime boundary. #59 still needs the remaining hook work.

No public API, DB, dependency, runner or logging schema change. No Git push,
new branch/worktree, issue comment or hosted CI run.

## Next priorities

1. #59: synchronous cursor/desktop checks and physical ledger/queue gates on the
   hook callback. Preserve first-click routing and current desktop authority.
2. #50: native injection/guard work that cannot honor cancellation; retain
   uncertain pressed state and prove owner retirement before replacement.
3. #70: real endpoint-change acceptance with the pinned peer identity unchanged.
4. #15/#16/#20: remaining actual accessibility/DPI, full install/remove and signed
   release gates; #4/#5/#6/#7 still require their explicit hardware reports.

No further implementation task is started after this run's ten-minute cutoff.

## Paired application and verification

Both PCs now run g379, revision 5e2e7cd97b0d3280a0d542d7534f26dbd61c2cb1.
The frontend's 85 tests, native build and eight-file archive/installer provenance
checks passed. update verify accepted 27,930,473 bytes; B's receipt
6832b91dd48b9350e4e2d46b1b8787fe reports UPDATED at that revision. A installer
returned 0, and its installed agent SHA-256 matches the package.

Doctor reports healthy/CONNECTED and matched UI, agent, installed image and
recovery record. Paired collection at 20:11:19Z/20Z reports READY input health,
successful installations without recovery, and completed cleanup on both sides
(zero stages removed). Its 22 A and 49 B current-build events contain no
fault_/session_failed code. B's temporary network timeout/reacquisition occurred
while A was being replaced and is retained as such. Immediate launch-time
agent-unavailable preceded successful startup; no second installation was used.
This short window does not establish long-term or physical-input acceptance.

The ten remaining issues are #4, #5, #6, #7, #15, #16, #20, #50, #59 and #70.
None is closed solely from this user input report or these admission fixes.
Unsigned release, hosted CI, hardware fault/DPI/polling/accessibility matrices
and non-cancelable native-owner retirement remain unverified in this run.
No product source-of-truth contracts changed; only implementation and these
experiment/acceptance records changed. Changes remain on local main only.
