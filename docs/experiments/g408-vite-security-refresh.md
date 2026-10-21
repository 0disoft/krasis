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
