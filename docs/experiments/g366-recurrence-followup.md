# Source activation and reader termination after g366

Collection `61b4d586e5dc8d509e8744a297fb22a5` preserves the failed normal-use
trial. A records activation response timeouts at 2026-10-20 13:40:21Z and
14:11:38Z. Heartbeat attempts at 13:47:28Z and 14:02:45Z instead end because
the client response reader has terminated. A separate trust-freshness expiry
at 14:09:17Z reconnects after clean source retirement. B records remote EOFs
around those incidents, approximately two seconds earlier. Cross-host order
cannot be inferred without verifying clock synchronization.

The source service receipt joins native capture, sender and interrupt owners
for all five exits. Four final retirement deliveries fail. The last source
exit is at 14:11:39Z; the next agent start at 16:19:25Z follows the operator's
manual recovery. These observations do not establish the cause of every
reader termination or the host scheduling delays.

Priority by user impact and implementation cost:

1. Correct premature activation failure between the one-second client cap and
   existing two-second owner bound. Implemented with a 1200 ms exact-reply
   regression through `CaptureSource.ActivateProfile`.
2. Preserve the response reader's terminal cause, currently collapsed into
   `reader_ended`, to distinguish transport failure from invalid packets or
   native retirement. Implemented with fixed, sanitized terminal categories;
   the existing failure and retirement behavior is unchanged.
3. Investigate the observed scheduling stalls and trust-refresh delays using
   the next failed trial; no CPU or network cause is established yet.

Activation may now wait up to two seconds for its exact ACK. An already active
or cursor-prepared owner retains the earlier deadline derived from its last
successful request start. Caller deadlines remain binding. Missing ACKs still
clear the epoch and close the channel without replay; expired owners cannot
send activation. Other input commands retain one second. Worker receive
deadlines, authorization, desktop guards and native cleanup are unchanged.

Regression coverage includes delayed exact replies in standby, active and
cursor states; missing replies at the active/cursor deadline; shorter caller
deadlines; expired owners; and unchanged non-activation command limits.
Installed programs remain g366 until a separately authorized application.
The distinct manual-trial package uses generation 367 to preserve provenance.
This addresses one reproduced failure condition; sustained paired stability
is unverified. No public API, DB, dependency, hygiene or CI runner changes.

Local verification passes: inputworker, residentagent, agent, peer, productlog
and localapi tests; vet with the repository's existing unsafeptr exclusion;
and whitespace checks. Terminal-category tests retain cleanup priority and
never export arbitrary error strings. The distinct package and installed
acceptance results are recorded separately when performed.

## Verified g367 trial package

Package revision `13c8f375689834018c97356509f2e2a2f5fee344` passes all
85 frontend tests, native UI build, installer provenance, package transfer
roundtrip, icon and eight-file checks. Offline CLI update verification returns
valid/ok for generation 367, 27777535 bytes. The package is a local manual
two-PC trial artifact, not a public release.

At 16:31Z A doctor still reports healthy/CONNECTED to B with running and
installed g366. G367 has not been installed on either PC; installation awaits
the existing operator approval gate. The next priority is applying the trial
and observing ordinary use, distinguishing any remaining reader termination
from the corrected premature activation timeout. No Git push was performed.
