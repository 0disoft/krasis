# ADR 0043: Hide the active outbound capture cursor

Status: Accepted

The user confirmed generation 14 hides both destination cursors during round trips
with A's mouse or touchpad. B's own touchpad controlling A still leaves B's cursor
visible. This is the outbound capture window, not the idle return shield.

The capture window retains its normal arrow class cursor while inactive. Once
activation succeeds, its owner thread calls SetCursor(NULL). While active it also
handles WM_SETCURSOR, reapplies the null cursor and stops default arrow handling.
Deactivation hides the capture window as before. No global cursor replacement,
ShowCursor counter, focus, wheel, hit-test or return-shield behavior is changed.

Mocked tests check inactive, repeated active and deactivated cursor requests.
Manual verification remains necessary for B-touchpad outbound, return, scrolling
and disconnect. The user-confirmed generation 14 behavior must remain intact.
Agent generation is 15.

Reference: [WM_SETCURSOR](https://learn.microsoft.com/en-us/windows/win32/menurc/wm-setcursor).

## Keep cursor painting outside input authorization

The owned capture window's `WM_SETCURSOR` callback and initial null-cursor paint
use the already-activated owner's atomic state and cancellation only. They do
not call `sourceLoginDesktopCheck` / `Scope.Do`: the fixed capture scope runs a
native desktop-activity query, including while Windows is delivering
messages inside a foreground transition. Inactive/returning owners make no
such queries either. Cursor painting emits no input, activates no window and
does not grant or renew a lease. Activation, geometry admission, desktop
selection and wheel forwarding keep their existing live checks.

Regression tests verify repeated active painting with an unusable input scope,
rejection of activation on that same scope, and no painting after deactivation,
cancellation or native failure. This removes one scope check per cursor callback;
the effect on physical handoff latency still requires a new two-PC run.
