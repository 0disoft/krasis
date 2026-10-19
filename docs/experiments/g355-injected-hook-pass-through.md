# g355: Avoid physical desktop work for ignored injected hook events

g354's attended movement trial clearly improved perceived stutter, but retained
receiver ordinary dispatch tails of 78778us and 103852us. The long movement tag's
worst operation includes a 27613us native submission and 72882us until the client
observes its first ACK bytes. These are overlapping local spans, not additive
or proof that low-level hook work accounts for the whole delay.

Static inspection finds that source low-level callbacks perform
sourceLoginDesktopCheck before excluding already-ignored injected events. In
protected capture this enters Scope.Do and its current desktop/authority checks.
The Windows low-level hook executes by messaging its installing thread; work
there is therefore relevant to the synchronous native input path. The exact
contribution to the retained tail is unmeasured.

The bounded change bypasses physical desktop work only for keyboard injection
already excluded by the mapper and tagged non-wheel mouse events already excluded
from physical routing. On the owner thread, it still invalidates pending passive
presentation, propagates earlier cleanup failures and updates injected mouse
position. If a shield is visible, current-desktop validation still precedes its
synchronous native hide. No injection, physical suppression, boundary intent or
lease authority is added to this branch. Actual physical events are unchanged.

Tagged wheels retain the original capture-window suppression; untagged synthesized
wheels retain precision-touchpad routing. Tags are not authentication and are not
used to admit input. The injection worker independently retains every request's
authority, desktop, sequence, binding, deadline and matching ACK rules.

Callbacks are extracted into one owner-local factory for direct tests; native
registration remains the same singleton keyboard/mouse trampolines and thread-ID
owner map. No new timer, callback slot, per-event task or process is introduced.
Preparation lifetime, lease timeout, transport and g354 ACK policy are unchanged.

Focused tests call the actual owner callbacks with injected/tagged events while
the physical gate is held, verify no boundary emission or desktop check, and
verify pointer/presentation invalidation. Other tests preserve physical checking,
touchpad-wheel routing, owned tagged-wheel suppression, visible-shield validation,
restoration and retained cleanup failures. Device benefit remains unverified
until both PCs run this build and the attended movement trial is collected.

First-entry preparation remains a separate unresolved cost: the g354 long tag
had no prepared candidate and about 232ms source request-to-grant. This patch
does not extend warm-child lifetime or claim to fix cold entry.

Primary reference: [Microsoft LowLevelMouseProc execution model](https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelmouseproc).
