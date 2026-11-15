# g539: joined active capture retirement and pollable native ACK reads

## Source evidence and bounded changes

The g538 paired crossing review retained an A `installed_source_reader_retired`
event at 2026-11-15 04:04:18 KST, followed by `fault_source_hook_failed` and a
paired reconnect. The matching service retirement report was clean: command and
report OK, native capture/sender/interrupt joined. Service start times did not
change. That closed reader category means retired=true with nil reader error;
it is not itself proof of a native failure.

The capture client creates a lease lasting one hour minus one second, and the
worker bounds its run context to that expiry. The resident previously returned
ErrProtectedInput when an authenticated, clean retirement arrived while the
outbound SourceSession was active. Idle retirement already reused the session.
This active-retirement branch reproduces a reconnect from an otherwise clean
hourly retirement. The old generic error does not distinguish every possible
held-press/race branch in the real incident, so attribution is bounded accordingly.

`65d85e0b` handles only authenticated clean retirement after native owners and
client handles have joined. It atomically fences new grants, invalidates queued
forwarding and publishes one existing local-return request. The ordinary agent
return owner deactivates/revokes the outbound lease under the existing semantic
write coordinator. The resident waits at most the existing two-second source
cleanup budget for completion, then rebinds a freshly authenticated worker in the
same session. The sender's grant fence stays closed until replacement readiness.
There is no one-hour authority extension, new wire command or input replay.
Active control safely returns local at retirement; continuous control across
expiry is not claimed. Physical-held state, terminal sender, missing completion,
cancellation and uncertain cleanup still reject replacement/fail closed.

## B native reply transport

The g538 worst B dispatch (393.157ms) contained a child reply-ready-to-parent
first-read-return span of 337.190ms. Actual child handler was 26.968ms, with
23.335ms SendInput submission nested inside it. The 337ms span combines encoding/
writing, pipe transit and parent read resumption; it cannot identify which OS
scheduling or native write substep consumed all that time.

The production prepared native input owner uses crashguard.startCommand through
PrepareLocalInputCommandContext/Bind. startCommand used exec.Cmd.StdoutPipe,
which provides a synchronous Windows anonymous-pipe reader. Every waiting native
ACK can therefore occupy an OS thread in ReadFile. The stderr path had already
been made pollable; the native reply path had not.

The Windows parent now reads through an overlapped private pipe using Go's IOCP
poller. Waiting goroutines yield without one synchronous blocked OS thread per
waiting child. The child retains its normal synchronous inherited stdout writer;
only the parent read handle is overlapped. The same buffered reader retains guard
and input ACKs, including prefetched suffixes. Framing, binding/sequence checks,
process ownership, abort-on-unknown outcome and independent guardian remain.

The parent's duplicate writer is closed after successful Start and every startup
failure; otherwise exit could fail to produce EOF. Reader cleanup stays owned by
Process. The existing 128-bit random private user/SYSTEM ACL and remote-client
rejection are reused. Handle count remains one parent reader plus one child
writer per reply pipe. No extra goroutine or ACK exchange is added per command.
Linux keeps its existing StdoutPipe implementation.

This removes a structural synchronous-read dependency; it does not establish
that the original entire 337ms was solely that dependency or guarantee a new
latency maximum. A fresh paired physical trial is required after installation.

## Validation and delivery

Focused tests verify active retirement requests exactly one revoke; completed
revoke cannot reopen a grant before readiness; replacement uses the same source
session; missing completion/cancellation stay fatal; held physical state and
terminal senders are preserved. Native stdout tests verify pollability through
read deadlines, failed-start EOF, complete 64KiB child output/EOF and cancellation
of an outstanding read by Close. The full crashguard suite also passed.

Uncached crashguard/inputworker/wininput/residentagent/agent/peer regressions,
scoped wininput/residentagent/crashguard race/checkptr tests and vet with the
existing -unsafeptr=false policy passed. PortablePair inputworker fixtures were
excluded while the installed paired service is live; real child ACK/guardian
retirement and parent-death fixtures in crashguard were included. No physical
input is synthesized by those fixtures. Native stdout tests separately passed.

Paired installation and remaining delivery limits are recorded below when
complete. Rollback uses the verified installer workflow
with a new generation containing the previous source; existing generations are
never overwritten. No public release, remote push or CI runner change is needed.
