# g403: bound portable protected-input inspection (#50)

The portable resident called InspectProtectedInput synchronously inside the
press gate. The same native read ran in the monitor that runner shutdown joins,
and in the outer protected-recovery wait. A native desktop/process query that
ignores cancellation could therefore retain input admission or session shutdown
even after SendInput had moved to separate child processes.

Those read-only inspections now use the caller's remaining context, capped at
the existing two-second injection budget. Each observation owner serializes its
queries; at most four native queries exist per process across replacements.
Expired queries retain both their admission gate and capacity until they really
return and close their locally owned native handles. No input/native command is
moved into this goroutine. There is no cached clear decision or replay: every
admitted press obtains a fresh inspection and rechecks cancellation after its
result. Canceled gate waiters do not launch replacement queries. Panic/Goexit returns a
fixed unavailable result and releases capacity, without exposing a payload.

Inspection failure before guard preparation has InjectionNotApplied certainty.
Release bypasses this read-only gate, allowing held-input cleanup while a query
is still outstanding. Monitor cancellation joins its bounded caller rather
than the native task; unknown inspection failure is not logged as an observed
protected screen. Installed service input retains its existing authority path
and does not add per-press inspection tasks.

A distinct typed read-only deadline can preserve the user's connection intent
after the runner has completed its other cleanup. Recovery waits for native
inspection slots to empty before starting a fresh runner/authorization. It does
not reuse a late result. Raw cleanup deadlines, other cleanup failures, partial
or unknown injection, unavailable/panicking inspectors and manual stop do not
qualify for this retry. A native task that never returns keeps RECONNECTING
fenced and remains bounded by process-wide capacity.

## Verification and remaining work

Residentagent, application and peer suites and agent build pass; residentagent
vet and diff checks pass. Virtual-time tests cover uncooperative inspection,
expired press with no guard/native submission, independent release, ignored late
clear, fresh subsequent query, capacity retained across expired owners,
panic/Goexit, monitor cancellation, exact read-only owner retirement before
retry, and refusal to hide cleanup/uncertain-input failures or manual stop.
These are controlled read-only/adapter tests, not native held-key or Winlogon
acceptance. Physical input and latency claims require their separate device
evidence; race detection was not run with CGO disabled.

#50 remains open for the rest of the production call graph and bounded native
presence/cleanup acceptance. #59 source hook/native cursor and semantic gates
remain separate. No public API, DB, runner, dependency, firewall, signing,
branch/worktree, remote ref or issue-comment changes. Generation 403 identifies
the authorized A/B update; application evidence is appended after verification.

## Pre-application diagnostic correction (g404)

Final inspection found the two new fixed inspection failure/reconnecting event
codes missing from productlog's allow-list. Add both so persisted/sanitized
support evidence retains the cause without accepting arbitrary details. The
existing export privacy test now covers both codes. Productlog tests/vet pass.
The g403 artifact was built and checked but was never installed or sent; g404
supersedes it before device application. This is completion of the same unit,
not a second behavior change or a claim of real-device timeout reproduction.

## g404 application

UI 85 tests, native builds, embedded installer provenance and eight-file bundle
checks pass. Offline verification accepted g404 revision
780fceb9796f629c6ed9bdd293d9fd53e07ad7d8 and 28,265,219 bytes. g403 was not applied.

A's first install at 00:33:02Z exited 4: replace/file_move, Win32 5, recovery
restored. The g402 installed-agent digest and running restored service were
verified before retry; the original receipt is retained locally in
.cache/g404-first-install-restored.json. The subsequent install exited 0 at
00:33:27Z, complete/result 0, no recovery needed. Its installed agent SHA-256
matches g404. The installed-input cursor/same-point placement and recovery
shutdown smoke exited 0. Recovery entrypoints report recovery_ready. This
repeats g398's first-attempt access denial; the exact file-move cause remains
unproven and should be investigated under #16 before adding blind retries.

B update receipt 5bb718b8a1f1c82c2c97c8a16d68318d reports UPDATED at g404, and its
installation receipt at 00:34:05Z is complete/result 0. A was explicitly
reconnected after B replacement. Initial collection
afb1d3a33603919bcca48e1fcc86a3b9 captures startup in progress; it does not prove
that B's source capture or the new connection was ready.

Collection abf7eb70ea0241ef9db8242232d74f55 at 00:36:17Z/19Z confirms READY
services and no current-build fault/startup-failed code. A has capture readiness
and remains CONNECTING; B has agent_ready and launch_record_ready but no
connect_started or protected source readiness. Its update applied successfully,
but automatic connection resumption is not verified. Authenticated app-open
request 22a15d9da94d6f7acefd010f33db537c reports window_ready. The user was asked
to initiate B's connection once; no manual-stop preference was overwritten,
remote shell invented or blind remote input injected. Do not report both PCs
connected until fresh evidence proves it. No extra issue was closed.

## Continuation priorities

1. Finish the pending B connection observation after the requested user action;
   inspect automatic-resume state if it still does not request a connection.
2. Investigate the repeated first-install replace/file_move access denial under
   #16 using the retained g398/g404 receipts. Restoration is proven; the exact
   process/handle/ACL cause is not yet proven.
3. #50 still has Client.performResponse's direct pinned-presence query under the
   serial client gate. Any bounding change must retain native-query handle
   ownership and preserve TestSlowInputAuthorizationDoesNotStarveWorkerHeartbeat,
   including its two real heartbeat writes while full authorization is stalled.
   Merely skipping heartbeats behind the stalled full query is not a solution.
4. #59 retains native desktop/cursor callback work and physical/producer gates.

No new implementation unit was begun after the ten-minute cutoff. Open issues
remain #4/#5/#6/#7/#15/#16/#20/#50/#59/#70. Home-pair identity and signing-service
availability are still awaiting the earlier user answers. Push, issue comments,
hosted CI and remote branch changes remain unperformed.
