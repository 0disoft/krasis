# g428: distinguish unobserved registration from a confirmed mismatch

An actual g427 screenshot shows an app recovery registration warning while a
Windows Security firewall permission dialog blocks UI interaction. Independent
CLI recovery-status reports task ENABLED; the task target matches g427. This
does not identify the exact earlier GUI read error or prove every registration
artifact healthy. The screenshot is not accepted as an unobstructed UI review.
No security-dialog input or firewall setting change is performed.

AppSettings currently maps both an OS registration query error and a confirmed
registration mismatch to startup_failed/recovery_failed. A read timeout or
access failure therefore claims a failed registration without observing one.
A new injected-query regression fails both query-error cases on the original
source (full focused result 0.169s); mismatch, rollback-unknown and verified
cases preserve their existing meanings.

g428 projects startup_query_failed/recovery_query_failed for read errors. The
existing generic settings-verification guidance handles these outcomes without
showing raw errors. Confirmed mismatches keep their registration-failed codes.
Known saved preferences remain known and the frontend bridge preserves their
automatic-update guidance; an OS query error cannot rewrite user preferences.
Rollback uncertainty still takes precedence as rollback_failed. No read path
writes registry/task state, clears failed-write latches or grants new authority.

The full Windows/desktop workspacehost suite passes in 0.449s and all 86
frontend tests pass. The new frontend case verifies allowlisted projection,
raw-error exclusion and retained automatic-update guidance. This fixes a proven
classification defect; it does not add retry loops or claim the earlier GUI
warning's exact cause. Opening Settings already provides a new read attempt.

The local UI projection adds two bounded code values; old/mismatched frontends
retain their existing unknown-code fallback. No agent/network wire schema, DB,
dependency, runner, input path, signing, firewall or repository-hygiene change.
Product-control documentation records the distinction. Native packaging and
application results follow separately. No push or issue comment.
