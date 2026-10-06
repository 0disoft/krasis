# Source interaction feedback and explicit keyboard mode matching

Status: Accepted for local implementation; two-PC checks pending.

## Decision

Saved foreground restoration requires both the original process/thread and a
unique property on that window instance. The property carries no allocated
native resource or user content. It is removed on return, rollback and capture
shutdown. A reused HWND loses the original property's proof. Failure to install
or verify the property, including UIPI rejection, skips saved-window restoration.
The checks run again after a responsiveness probe and before a fallback request.
They do not make the final identity check and foreground request atomic.

Boundary admission remains owned by the existing input controller. Its observer
publishes only LOCAL_MENU, MOUSE_CAPTURE, PRESSED_INPUT or MOVE_SIZE to the local
status API and desktop bridge. A notice expires after three seconds and clears
on successful admission/activation. Retired runners and non-connected states
cannot publish a current notice. No key, button, window identity or content is
included in that status.

The desktop GUI samples Caps Lock, Num Lock and, when available, the foreground
Korean IME's open state on its message thread. Caps/Num are known only while
the Krasis GUI owns foreground: GetKeyState is queue-relative, so a background
GUI must not advertise its old queue state as current global state. The IME query is limited to 20ms.
An authenticated management-channel extension publishes these observations;
the age since sampling plus the age since receipt must be below six seconds.
Missing, unsupported, malformed or expired observations remain UNKNOWN. This is
display metadata, never input authority. The local API accepts only the three
mode values and retains no keyboard contents or mode history in diagnostics.

Matching is an explicit, one-shot action on the PC where the user clicks it. It
matches that PC's Caps/Num states to a fresh peer observation. It requires the
Krasis window to remain foreground, no held modifier/lock key and an unexpired
GUI operation. It never issues a remote input command, changes modes on boundary
crossing or persists an enabled synchronization setting. Partial/uncertain
submission attempts only releases; it never repeats a potentially applied toggle.
The GUI reports a request, then subsequent observations show the resulting state.
Hangul mode is per application and is excluded from matching.

## Validation and remaining checks

Unit checks cover recycled windows, identity loss during probing, notice expiry
and stale-runner callbacks, malformed modes, legacy management messages, sample
expiry, held keys, foreign foreground and partial-toggle cleanup. Frontend checks
cover peer identity and output narrowing. Packaging does not activate either PC.
Real checks must cover fullscreen video, context menus, PIN re-entry, displayed
lock states and one-shot Caps/Num matching on both PCs. Modern TSF IME observation
still requires field verification. Caps/Num matching requires both Krasis GUIs
to be foreground on their respective PCs.

## References

- [Window property functions](https://learn.microsoft.com/en-us/windows/win32/winmsg/window-property-functions)
- [SetProp and UIPI](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setpropw)
- [GetKeyState message-thread semantics](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getkeystate)
