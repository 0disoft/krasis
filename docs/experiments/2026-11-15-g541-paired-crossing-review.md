# g541 paired trial: capture improved, main-runtime and grant tails remain

The user completed the requested physical crossings and reported stutter.
Collection `0d316fb530aac96e8844a4107e32a07a` contains exact g541 revision
`05ef45d41bdbe2fc23558e11ef8481a0d93f6bfb` on both notebooks. Live doctor
is healthy/CONNECTED; CLI/agent/UI/launch/protected installation match. B is
ONLINE at the same generation/revision, with transfer IDLE. No new install or
source change is needed to collect this trial.

## Timing result

Twelve source/receiver profiles match uniquely by exchange tag, all complete/OK,
covering 498 receiver frames. A profile timestamps run from 12:19:31.2009526Z
to 12:20:12.4768471Z. Profiles are capped at 12 attempts per 120-second window;
this is the recorded sample, not an unlimited census of all user motion.

| Local interval | g540 median / max ms, 10 pairs | g541 median / max ms, 12 pairs |
| --- | ---: | ---: |
| A capture activation | 51.917 / 135.798 | 20.043 / 39.062 |
| A foreground request/exchange | 33.817 / 57.962 | 8.224 / 29.155 |
| A post to capture-owner handling | 1.149 / 17.928 | 0 / 0.559 |
| A request to grant | 47.914 / 94.111 | 58.567 / 107.707 |
| B worker readiness wait | 24.695 / 27.862 | 23.827 / 27.165 |
| B first input dispatch | 2.271 / 8.523 | 1.775 / 3.016 |
| B worst input dispatch per entry | 3.471 / 71.448 | 2.782 / 6.771 |

Capture activation and owner resumption are lower in this sample. This does
not establish a controlled causal effect: CPU pressure, workload, sample counts,
observation durations and receiver frame counts differ (1,933 versus 498).
Measured zero means below the microsecond recording resolution, not missing.

Activation-command authority scheduling wait has median/max 0/0.525ms;
the worker's corresponding read-authority wait is at most 0.567ms. Authority
native time reaches 0.569ms. No source-service-query-deadline-scheduled expiry
was retained. A successful heartbeat client-authority 100ms-to-1s timing bucket
at 12:16:39.6268371Z preceded these physical trial profiles; the absence of a
deadline failure does not mean all client-side checks became instantaneous.

The worst 39.062ms activation includes 36.311ms worker activation and 29.155ms
foreground exchange. Same-command request delivery is 0.097ms, reply delivery
0.077ms and caller resumption 1.182ms. Across entries, caller resumption reaches
6.012ms and reply delivery 2.353ms. These nested spans cannot be added together
or described as one slow native call.

## Remaining stutter signals

The largest recorded entry wait is now A request-to-grant, 107.707ms. For that
same exchange B read-to-grant-written is 32.188ms, worker readiness 23.579ms,
and sampled TCP smoothed RTT 66.027/66.140ms. A post-read control queue and
grant handler admission are zero at recording resolution. This points the
next investigation toward request/reply transit, application read resumption
and B's readiness path, rather than the old capture-owner queue. It does not
identify the entire wait as network transit. Across the trial TCP sampled RTT
median/max is 23.617/95.669ms (previously 14.508/37.307ms); retained sampled
retransmitted-byte and timeout-episode deltas are zero.

A's minute ending 12:20:23.5864574Z overlaps the trial and records session
timer lateness 467.673ms, independent sampler lateness 1,081.255ms, source queue
residence 29.236ms, and one slow control dispatch 88.212ms. Its GC pause
histogram is `[4,2,1,1,2,0]`, runnable histogram `[9821,238,305,29,0,0]`.
The GC aggregate therefore includes two entries in the <=1s upper-bound bucket.
Aggregation assigns source histogram buckets by their upper bound: these are
not exact 100ms-to-1s pause measurements. GC stop time can also reflect waiting
for execution; the counters do not identify an allocating component, offending
process, CPU starvation cause or the exact pause overlapping a given entry.
The slow control dispatch is one separate matched operation, not the entire
107.707ms grant wait. B's observed timer/sampler maxima are 6.855/10.061ms.

FirstOfferSinceBegin reaches 180.431ms and cadence gaps also exist, but both can
include intentional idle/source input availability. They are not measured
input-to-display latency. Neither the 467ms timer maximum nor 1.081s sampler
maximum means the user's cursor was continuously frozen for that duration.

The data supports investigating the normal-priority main agent's runtime/GC
and receive path next. It does not justify promoting the entire service/GUI,
changing timeout policy, blaming all delay on Wi-Fi, or another speculative
foreground API rewrite. New scheduling/GC traces are required for attribution.

## Fault and recovery result

Both devices retain the same shared connection for 247.275s / 248.602s after
their respective CONNECTED events. There are no retained postconnection faults,
reconnects, source reader retirements, joined worker refreshes or poisoning.
Both service/input states remain READY with grants PRESENT. Service start times
and recovery last-failure entries match the immediate postinstallation evidence;
no new error-1066 stop or administrator-required repair is evidenced.

This is approximately 4.1 minutes, not long-duration stability proof. B's
installation-time acquisition failures preceded this connection and are not
counted as new trial incidents. Historical control.previous logs remain incomplete;
current matching-build agent logs and service/recovery ledgers were inspected.

## Verification and scope

Checked exact builds, diagnostic SHA256 hashes, same connection, event deduplication,
unique exchange tags, all matched outcomes, frame histogram sums, measured-value
masks/zero handling, authority/query phase meaning and unchanged service/recovery
ledgers. The adjacent JSON retains all twelve compact samples, matching worst
command details, runtime histograms and baseline statistics.

Source tests, native smoke, rebuild, install, remote CI and push are skipped:
this step records diagnostics only and changes no runtime code. No API, DB,
dependency, hygiene, runner or source-of-truth contract changes. Do not subtract
A/B wall clocks or add nested intervals. Physical stutter remains reported and
its residual cause is not yet fully attributed.
