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
