# g380: isolate source diagnostics and reject capture during authority close

## #59 changes

Source authority checks on low-level callbacks now use TryRLock. An authority
being closed denies new capture immediately instead of waiting behind native
SCM/WTS checks held by existing readers. Every admitted reader still performs
the full fresh authorization check while pinning the handles. Close still joins
those readers before releasing handles. Neither console nor desktop activity
is cached and no authority is inferred from diagnostic observations.

Production agent sessions now isolate optional source pipeline/queue observers
behind a 256-event nonblocking relay. Native source hooks also isolate pipeline
and cursor-presentation reports. A slow callback no longer holds input routing,
source sending or synchronous first-click cursor hiding. Real input stays in
its existing ordered queue; diagnostics alone may be dropped. A process has at
most four relay consumers, including any consumers stuck across owner changes.
Teardown never waits for these optional consumers. Consumer panic is contained.

The additive runtime summary field sourceObservationDroppedTotal marks lost or
uncertain diagnostic samples, including producer/close races and consumer panic.
Queue residence is captured at the original event time. Pipeline and queue
consumers receive their own events without duplication. No input contents,
coordinates or authority data enter relay samples.

## Validation

Passed the complete inputworker, wininput, agent, productlog and residentagent
suites using repository-local caches and GOPROXY=off. Focused tests additionally
cover a closing proof, four permanently blocked observers, ten attempted owner
replacements, a full 256-sample queue, allocation-free callback handoff, actual
physical key-down/up routing and sender order, preserved diagnostic outcomes,
separate queue consumers, reporter panic and nil observers. A focused rerun
passed after replacing numeric event tags with named constants and explicitly
marking close-racing samples as uncertain.

## Remaining scope

#59 remains open: physical/state/queue semantic gates and immediate native
desktop/authority and synchronous cursor-hide calls still exist. Suppression
must not become a stale authorization cache, and a first click must not reach
the passive shield. Optional diagnostics are now isolated; these mandatory
native decisions still need evidence and a safe bounded design. #50 separately
retains native operations that ignore cancellation. No claimed input/latency,
hardware fault, home-pair, DPI/accessibility or signed-release acceptance is
added by these controlled tests.

No public control API, DB, dependency, firewall or runner change. Runtime
diagnostics gain one optional aggregate field. No push or issue comment.
