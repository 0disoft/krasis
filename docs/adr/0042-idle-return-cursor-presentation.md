# ADR 0042: Hide the destination cursor after a successful return

Status: Accepted

## Behavior

While the connection remains active, returning from B to A using A's mouse
places a passive 24px cursor shield at B's idle pointer. This is presentation,
not disconnect or a new control lease. It is prepared before return and shown
only after the exact revoke write succeeds. Held input or a failed write never
shows it. Re-entry invalidates pending presentation before cursor placement.

The layered, non-activating topmost window uses alpha 1/255 (not zero or click
through) and handles WM_SETCURSOR with a null cursor. It does not take focus,
capture input, change system cursor resources or modify ShowCursor counts.
This intercepts hit testing at the idle position; arbitrary application hover
state is not guaranteed to clear until the application processes mouse leave.

The existing low-level-hook thread owns the window. Any keyboard/mouse hook
event, including injected input, invalidates pending presentation and hides the
window synchronously before the existing routing / CallNextHookEx. It does not
consume or replay the first input. Generation and cursor-position checks reject
stale requests. Shutdown destroys the window. One process-wide window callback
and class are reused; no callback is allocated on each return.

## Validation and limits

Mocked native tests cover stale completion, queued requests, moved pointer,
re-entry and immediate first-input restoration. Peer tests cover successful
return and held-input rejection. Native window creation is best effort; if it
fails, the cursor remains visible rather than breaking input control.

Manual two-PC checks are required: repeated A-mouse round trips, B's own first
move/click/wheel/key, web hover, light/dark backgrounds, re-entry and disconnect.
Near-transparent presentation can have compositor-specific visual differences.
The existing low-level-hook reliability constraints still apply. Generation 14.

References: [WM_SETCURSOR](https://learn.microsoft.com/en-us/windows/win32/menurc/wm-setcursor),
[layered hit testing](https://learn.microsoft.com/en-us/windows/win32/winmsg/window-features).

## Credential UI without a desktop switch

The installed backend watches cursor protection while its ordinary source owner
is live. A credential broker on Default need not emit a desktop-switch event.
When protection appears, idle-owner replacement joins the ordinary owner before
starting authenticated protected capture. Subsequent returns reuse that owner's
shield rather than starting a passive presentation child on every return.

This promotion uses the existing activation, held-input and pending-return gate;
it waits while that gate refuses replacement. Foreground changes do not demote a
protected owner. Watchers are joined with their exact source attempt, and stale
attempts cannot pause a successor. Actual desktop transitions retain their
existing lifecycle policy. Device checks must still measure first-promotion and
repeated-return latency separately.
