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
result. A canceled waiter cannot submit a late query. Panic/Goexit returns a
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
