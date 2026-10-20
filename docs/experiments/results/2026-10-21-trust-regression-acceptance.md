# Trust contention regression acceptance — 2026-10-21

Issue #90 concerns the g54 trust monitor disconnecting an otherwise active
session on short read contention. Its implementation fixes are already on local
main: shared reader locks (`8869e21`), bounded freshness/revocation handling,
timely-verdict delivery and reduced filesystem contention (`df8ba4e`).

## Windows verification

`TestTrustReadAcrossProcessesPreservesRevocation` now launches a separate copy
of the Windows test executable. While that process holds the actual shared file
lock, the parent validates the certificate successfully and cannot acquire an
exclusive revocation lock. After the reader exits, revocation succeeds and the
next certificate validation rejects the peer. A 15-second process deadline and
deferred process cleanup bound failures. No product binaries are changed.

The new test passed with `-count=1`. Focused truststore/residentagent tests also
passed for concurrent readers, read-only Windows sharing, revocation, transient
contention recovery, persistent contention expiry, delayed observer delivery and
trace phase attribution. The same command passed peer/wininput stream and queue
regressions. Repository-local Go caches and `GOPROXY=off` were used.

## Paired device observation

Collection `8cce9dc8685eb917470c43549b4ccdf1` was created at
2026-10-20 17:32:19Z on A and 17:32:18Z on B (2026-10-21 in Korea).
Both report input health READY and g368 revision
`444d0e44e73ecddc063ee53ebbb5f2cead20167b`.

Filtering all exported events to that revision gives:

| Observation | A | B |
| --- | ---: | ---: |
| Runtime summaries | 32 | 31 |
| Queued source actions | 4,748 | 0 |
| Completed injections | 0 | 3,838 |
| Maximum source queue depth | 3 | 0 |
| Queue overflow events | 0 | 0 |
| Trust expiry/failure events | 0 | 0 |

A records slow `prelock_lockfile_open` and `store_read_bytes` phases, including
the >=1-second bucket. Lock acquisition, prelock and store-read timings remain
separate. These observations locate delays without identifying the responsible
Windows process or security product. Raw input and personal paths are omitted.

## Disposition and remaining fault

The historical trust-contention defect has regression coverage and paired
active-input evidence without trust-triggered disconnects; close #90 as fixed.
This does not declare the whole connection stable: A had source heartbeat IPC
failures at 17:18:32Z, 17:28:46Z and 17:29:17Z, plus a network heartbeat timeout
at 17:26:19Z. The service's source reports preserve joined capture/sender/
interrupt owners, with failed retirement delivery for three exits. These are
separate from trust validation and must remain under investigation.

No new generation, device replacement, dependency/API/DB/runner change, Git
push, hosted CI or signed release is part of this acceptance. Original paired
snapshots stay local. Hardware polling and complete fault-matrix acceptance
remain in #7 and #6.
