# g408: PR #92 Vite patch refresh

Applied the exact lockfile diff from PR #92 head
75e8af312bc867f20112ce13b9262bf871805d1b to local main, preserving PR #91's
devalue 5.9.4 and source-map-js 1.2.2 entries. Vite changes from 8.3.0 to 8.3.3;
its compatible Rolldown dependency/bindings change from 1.2.9 to 1.2.14 and
@oxc-project/types from 0.150.0 to 0.154.0. Root major ranges remain unchanged.

Vite's version/SHA-512 integrity was checked against registry.npmjs.org. Its
official v8.3.3 release notes report the bundled launch-editor 2.14.2 update and
dev-server serving/path fixes. These upstream fixes were not reproduced through
an exploit locally; no untrusted filename or command was executed.
Source: https://github.com/vitejs/vite/releases/tag/v8.3.3

Locked npm ci --ignore-scripts --no-audit --no-fund installation, UI 85 tests,
native-mode Vite build and diff checks pass on Node 24.18.0. npm ls confirms
Vite 8.3.3/Rolldown 1.2.14 and the retained PR #91 versions. The generated JS
changes from index-DNWypHGd.js (160,778 bytes) to index-CoO_Opjq.js (160,830 bytes);
CSS name/size remain the same. Generation 408 identifies one paired bundle of
both dependency units, with device application recorded after verification.

No new dependency, public API/DB, input algorithm, runner, firewall or signing
policy change. No separate native-input/race/physical matrix or hosted CI rerun
for the dependency-only code change. PR #92 is closed without a comment after
local incorporation; it is not remotely merged. No push or remote Git ref update.
Dependency Dashboard #1 is retained as explicitly requested.

## Paired application

Native builds, embedded-installer provenance and eight-file bundle checks pass.
Offline verification accepted g408 revision
dc3d9cc8cb89af1d23ab583adba09904142c37a3, 28,256,819 bytes. A's verified UI
replacement installed at 01:32:38Z with complete/result 0, no recovery needed;
installed-agent SHA-256 matches the bundle. The first B SEND returned
peer_unreachable without a job. Fresh authenticated status then showed B ONLINE
at g407 and transfer IDLE. A subsequent SEND queued the sole actual update
5ecbd16aeb3fdb1b60f04ffc84343095, which reports UPDATED at g408. B installation
at 01:33:57Z reports complete/result 0, no recovery needed. No installer retry
or manual reconnect was required.

Collection c526d5eb4f2687e95c2732f23316d5dd overlapped B startup and contains no
completed connection proof. Fresh collection ffa73f07777a6b665466d27943d197e2 at
01:34:48Z/49Z contains 33 A / 32 B current-build events, CONNECTED on both,
READY service/input health, and no current-build fault/startup-failed event.
A's fresh doctor independently confirms healthy CONNECTED, matched CLI/UI/agent/
installed revisions, verified launch record and completed stage cleanup.
Physical held-input, power, DHCP, DPI/accessibility, home-pair and signed-beta
acceptance are not inferred. The known singleton conflict means no extra
standalone installed-input smoke was attempted alongside the normal resident.

PR #91 is CLOSED at local incorporation 105340f; PR #92 is CLOSED at dc3d9cc.
Both closures were checked with mergedAt null and their reviewed head hashes
unchanged. No comments, pushes or remote Git ref updates. Final remote read at
01:34Z has no open PRs and retains Dashboard #1 plus the ten unresolved issues
#4/#5/#6/#7/#15/#16/#20/#50/#59/#70. No further implementation was started after
the ten-minute cutoff; the two dependency units were completed sequentially.

## Next evidence-backed work

The source reader/watchdog still calls both proof.checkSource[Heartbeat] and
channel.authority.checkSource[Heartbeat] directly in source_worker_windows.go.
Their shutdown joins and bootstrap_windows.go's deferred proof/control closure
need coordinated ownership: bounding only the monitor while closing a still-used
native handle would be incorrect. SourceHook's low-level callback also reaches
Scope.Do -> desktopBinding.active's fresh UOI_IO query; desktop/thread affinity
must be preserved when addressing #59. These inspected paths are remaining audit
work, not newly reproduced failures or grounds to close #50/#59. Physical pair
identity/signing availability remain awaiting the earlier external evidence.
