# g528: Reuse primary TokenUser identity without caching trust decisions

The retained g527 reconnect has an explicit A-side freshness-expiry trigger
while prelock_lockfile_acl_query is active, followed by B remote EOF and retries.
That trace phase is GetSecurityInfo and/or execution delay around it. TokenUser
lookup precedes that phase, so the incident does NOT establish that SID lookup
itself caused the five-second expiry. Keep this distinction explicit.

The Windows credential validator repeatedly opens its process primary token,
gets TokenUser, converts the SID and closes native allocations. A normal trust
read visits the parent directory, private directory, lock file, trust document
and certificate. Repeating primary identity lookup adds native calls and heap
allocation although this application does not replace its primary TokenUser.
Thread impersonation was already ignored by OpenCurrentProcessToken.

Cache only a successful, nonempty process-user SID string, once per process.
A mutex serializes cold lookup; empty/failed results propagate and remain
retryable. No token handle, native descriptor, object ACL or trust verdict is
retained. Every object still receives fresh GetSecurityInfo/descriptor/principal,
reparse/hardlink, lock and certificate checks in the existing order. Revocation,
freshness, negative-result handling and the 5-second bound are unchanged.
A newly launched user/service/worker process obtains its own process identity.
If future code replaces the process primary TokenUser, this cache policy must
be revisited; token privilege changes do not change the cached user SID.

A local 100ms Windows microbenchmark measured warm cache 20.46ns/op, 0 B/op,
0 allocs/op; native lookup 7,104ns/op, 112 B/op, 2 allocs/op. This measures only
identity lookup, not overall trust latency. After warmup, repeated native identity
queries drop to zero; a representative five-validation read avoids five token
lookup sequences and ten Go allocations (560 bytes). ACL I/O remains unchanged.

Tests cover 32 concurrent cache users issuing one successful load, native/cached
SID equivalence, error/empty retry and subsequent reuse. Eight affected package
suites and standard vet for seven pass; wininput passes its existing CI
-unsafeptr=false policy. Dedicated reader/foreground/IPC race tests pass.
No public API, IPC, diagnostic JSON, DB, dependency or CI runner change.

This is a bounded optimization, not a fix for a hung GetSecurityInfo. g528 must
still be observed for a new trust expiry. Fail-closed freshness must never be
extended merely to suppress reconnect symptoms. Rollback is the g527 installer.

The SID cache concurrency/native-equivalence tests also pass with the configured
Windows CGO race toolchain, supplementing the agent's non-CGO test/vet run.
