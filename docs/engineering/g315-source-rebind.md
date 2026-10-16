# g315 source endpoint rebind

Paired request `dd0694a5d53715c24dfec27c3a220f13` preserves g314 A's failure
at 2026-10-16T04:17:59.196Z. The triggering source-hook has native error 2,
not a timeout. `installed_source_refresh_joined` occurred at 04:17:58.935Z;
the service recorded the retired source at 04:17:59.005Z. Native owners were
joined and the service remained READY. The agent restarted and reconnected.

The source retirement acknowledgement precedes the service's final worker,
report and handle cleanup. The serial source endpoint then unconditionally
slept another 200ms before recreating its pipe. The agent allowed a missing
pipe only 250ms from its first open: about 70ms of final cleanup plus 200ms
of backoff exceeds that budget. The observed agent failure came about 261ms
after refresh joined. This explains a reproducible missing-endpoint race;
g314 did not separately log pipe-open versus authentication failure stages.

Remove backoff only after a successfully completed source request and verified
cleanup. Rejected requests and ordinary failures retain backoff; uncertain
cleanup still stops the service. Source endpoint opens may now wait within
the existing two-second connect-and-authenticate deadline. Other installed
endpoints keep their 250ms missing-name grace; worker control opens keep zero
grace. Only missing/busy pre-connection opens wait. No authentication failure,
lease, command, activation or input is replayed. A connected endpoint must
still authenticate its actual kernel owner, protected image, installation
and service before any lease is written.

Source connection failures now emit allowlisted source-specific stage/native
codes. This distinguishes a missing pipe from a failed service identity check
without recording paths, identities, raw messages or input contents.

The native-pipe regression delays endpoint publication by 350ms. It fails with
the old 250ms budget (native error 2) and succeeds with the source rebind
budget. Cancellation, earlier caller deadlines and immediate access denial
remain covered. Local tests do not replace idle/lock/PIN device observation.
No public API, peer wire, database, dependency, ACL or runner changes.

Validation: native rebind regression failed before the fix and passed after;
focused pipe/rebind/retirement race tests passed. Inputworker, residentagent
and productlog package tests plus vet passed. No lock/PIN or long-idle physical
test was performed during this patch.
