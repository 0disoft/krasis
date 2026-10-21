# PR #91 frontend patch refresh

Applied the exact frontend/package-lock.json diff from PR #91 head
44d4dddb4ded561fdad694144c11339aec5bc838 to local main:

- devalue 5.9.2 -> 5.9.4, still MIT, through Svelte.
- source-map-js 1.2.1 -> 1.2.2, still BSD-3-Clause, through Vite/PostCSS.

Both are development/build dependencies. Their version and SHA-512 integrity
values were independently checked against registry.npmjs.org metadata before a
locked npm ci --ignore-scripts --no-audit --no-fund install. npm ls confirms the
intended resolved versions. Existing major ranges remain unchanged; no override
or permanent extra pin was added.

UI 85 tests, native-mode Vite build and git diff --check pass. Generated native
JS/CSS asset names and sizes match the preceding g407 build. No product source,
dependency dashboard, public API, DB, runner, firewall or signing policy change.
No separate native bundle was created for this build-tool-only unit; running
devices remain on verified g407. No Go/race/physical tests or hosted CI were
rerun because native input code did not change. Upstream security release-note
claims were not locally reproduced.

PR #91 is resolved by local incorporation, then closed without a comment. It is
not merged into remote main; no commit or branch was pushed and no remote Git
ref was updated. The local commit remains for the user's later batched push.
