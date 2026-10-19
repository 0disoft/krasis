# g357: Bounded source stream read-ahead

## Evidence and candidate

g356's attended trial is acceptable to the user, with 12 successful paired
handoffs, but retains A queue residence 40537us and control dispatch 145006us.
The first source activation has CaptureIPCResponse 100814us and the long movement
handoff later records SourceDeactivate 145005us. These are different operations;
they cannot be summed or treated as proof of one bottleneck. Later retained
collection `4f8abda61fc1efe3ec41e9795165b2e3` still has no fault or session-end
trigger and shows additional scheduling/control tails. Keep the working g356
baseline while testing changes locally.

Code inspection finds the source agent reads every fixed 64-byte packet directly
from the native pipe. Each native read creates and closes a completion event and
pins a new OVERLAPPED, including when multiple packets already occupy one native
pipe write. Advisory boundaries are already written in batches of up to 16;
actions and exact command replies share that ordered stream. This is avoidable
per-packet work, not proof it caused the user's earlier stutter or IPC tails.

The candidate reader has one 1024-byte buffer owned by one source client after
its unbuffered startup handshake. It never adds a goroutine, timer, input queue,
frame replay, authority cache or write batching. It requests at most one existing
advisory batch and returns a single packet immediately. All decoding, binding,
consecutive sequence, epoch and reply checks remain in their existing order.
Close still belongs to the channel owner; no buffer survives a replacement client.

Windows byte-mode pipes return available bytes and do not wait to fill the
requested buffer. See [Microsoft's named pipe modes](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-type-read-and-wait-modes)
and [ReadFile completion rules](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-readfile).
The native single-ACK test additionally verifies this property on this host.

## Local comparison before product adoption

`BenchmarkNativeSourcePacketStream` uses the same native pair, encoder, decoder,
packets and reverse-direction request token on both paths. The token bounds
producer read-ahead; each benchmark operation reads one whole batch. Three
one-second runs on Windows/amd64, AMD Ryzen 5 7430U:

| 16 packets per operation | direct | buffered candidate |
| --- | --- | --- |
| ns/op | 157553 / 157870 / 160250 | 56508 / 57732 / 56089 |
| native reads/op | 16 | 1 |
| allocations/op | 35 | 20 |
| allocated bytes/op | 1632 | 1152 |

Sequential single-packet subtests varied under host load, so they do not establish
regression. `BenchmarkNativeSourcePacketSinglePaired` alternates direct/buffered
order inside each run on the same connection. Three two-second runs:

| run | direct ns/request | buffered ns/request |
| --- | --- | --- |
| 1 | 51412 | 50867 |
| 2 | 50888 | 51464 |
| 3 | 55036 | 53081 |

This demonstrates amortized native reads under a burst, without a consistent
single-request slowdown in the paired harness. It does not measure cursor-display
latency or prove a human-visible benefit. Both paths allocate the same five
objects/192 bytes per single packet in the original harness.

Native tests pass for ordered batch delivery in one bounded read, an individual
ACK with no follow-up bytes, truncated-frame failure, and closing an outstanding
read. The candidate is not yet connected to the product source client at this
benchmark checkpoint. Production source, generation and both live devices remain
g356. No push, public release, CI, API, DB or runner change was made.

## Product adoption

After the benchmark checkpoint, source_client_windows.go uses this reader only
inside its existing serial packet goroutine, after the startup handshake and
before the unchanged sourceStart exchange. Worker command readers and every
other pipe user remain unbuffered. Generation 357 reserves a distinct artifact
for paired observation. The wire format, frame validation and error handling
are unchanged. This increases per-source read-ahead by at most 1024 bytes; there
is no unbounded backlog, new lifetime owner or change to writer serialization.
Native input is never injected by these transport tests.

Inputworker, localapi, agent and residentagent package tests pass, including all
native source-reader, reply, retirement, authority and cancellation tests.
`go vet -unsafeptr=false` passes for these four packages; the existing Win32
uintptr callback conversions are not claimed clean under unfiltered vet. The
focused native reader tests and both benchmarks also pass. No race run was
added; the new reader is local to the already serial source goroutine and is
never shared. Bugfix, desktop-app and memory-lifetime review procedures were
applied, including the performance/security checklists. Device benefit remains
unverified until a separately packaged build is applied and observed.

## Packaged artifact

Generation 357 at source revision `f567145670a3e532c29dd77921db53eeea8d8afd` passed
the ordinary workspace build: 85 frontend tests, native assets/UI build,
installer provenance, archive roundtrip and eight-file/icon bundle verification.
Offline update verification returns valid/ok and archive size 27754823 bytes.
Independent PowerShell SHA-256:

| File | SHA-256 |
| --- | --- |
| krasis.exe | aef6b2732510b6c00b20ea1b19247ed4f3f07ecba27e881036f8f0dcda4455b6 |
| krasis-agent.exe | 48d800a8f8caaba1837173cc33afb727325a3f72bb5685b0412a589e1aac91bf |
| krasis-cli.exe | c61c734d73029aa60219e6c61f1f587fb68946bc5e20dd38007bade4b2ef5124 |
| krasis-install.exe | a2fa17127a84b95d95fdf6171d43ab3c9deed55a2e257ce56cea860999304d6a |

B transfer `c6a0b407240ef5f0341dfa9d197322f1` reached UPDATED at this exact
generation/revision. A ordinary UI replacement was then requested. Device
health and the attended movement result are separate from these build/transfer
results; do not infer them from receipt of the archive.

## Old-version failure and completed rollout

While the local benchmark/build work continued, A's g356 baseline failed after
the attended trial. Collection `36c243f011e8f7049614ed6e851bf110` preserves the
old version before recovery. Its first trigger is source-hook /
fault_source_hook_failed at 2026-10-19 14:22:36.9782177Z. The heartbeat reported
installed_source_failure_authority just before it. The service itself remains
READY with unchanged service start time, not a stopped-service failure.
Source exit at 14:22:35.936769Z has command ok, reader cancelled, capture/sender/
interrupt joined, retirement clean_sent, report failed. Original native error
is unknown. Additional g356 source failures follow at 14:23 and 14:39. This
occurred before g357's 16:19Z B rollout and is not caused by the buffered reader.
The relationship between source lifetime, authorization, retirement and report
failure still requires investigation; buffered reads are not a claimed fix.

The old g356 UI's replacement request fails terminally at 16:20:06Z while its
agent is FAULTED. After saving the paired diagnosis, CLI safe-stop completes
and doctor confirms no responding agent. A second UI replacement request also
cannot progress with that missing agent. The exact old management UI PID/path
was checked and only that UI process was terminated; g357 was then opened
normally. Its protected-input update finishes automatically. No process tree
kill, manual service restart, security change or reboot was used.

The existing B profile connects again. A doctor now reports healthy/CONNECTED/
PROTECTED/CONFIGURED with matching g357 UI, agent, installed agent and launch
record. B inspection is ONLINE/IDLE at the exact g357 revision. Paired collection
`fab953111fc502b3e9156eb424a11133` is RECEIVED, both supports created at 16:22:49Z.
Both service and input-health records are READY. A install receipt is
16:22:27.9510022Z and B 16:19:33.5515601Z, both result 0 / complete /
recovery not_needed. Automatic installer process exit/stdout were not captured
separately from these receipts. No retained g357 fault/failed code or session-end
trigger is present in this short baseline; older control.previous.jsonl remains
incomplete on both PCs.

Human movement benefit and longer service/source lifetime remain unverified.
The next attended observation is the same short idle/continuous movement/returns
comparison, keeping the old g356 failure as a separate reliability investigation.
No further code, build, push, release or CI run is included in this evidence update.
