# ADR 0041: Latch uncertain session recovery

Status: Accepted

## Decision

An unexpected runner return, including an unexplained nil return, latches the
resident agent in FAULTED. Only SAFE_STOP is advertised; Connect and Pair reject
requests server-side. Refresh and Disconnect cannot reset the latch. The panel
explains that the agent must exit completely before being started again.

Clean requested cancellation and clean peer disconnect still return to LOCAL.
Transient network recovery remains inside the supervisor. Additional non-cancellation
worker failures and abnormal source-hook termination mark recovery unverified,
which prevents automatic retry even when the logical runtime reports LOCAL.
Product source close errors are retained alongside guard and session errors.

The latch is deliberately conservative: an unexpected setup failure returned by
the runner also requires restarting the agent. Ordinary preflight rejection does
not start the runner and does not fault the agent.

## Limits and validation

This does not certify native cleanup, detect a worker that never returns, or
cancel already queued native activation commands. Restart means confirmed process
exit, not merely closing the management window to the tray. A restart is not
proof that an operating-system cleanup failure has been repaired.

Tests cover fault persistence, server-side rejection, panel projection, clean
disconnect, late cleanup failure and supervisor retry rejection. Native two-PC
testing remains required for distribution validation. Agent generation is 13.
