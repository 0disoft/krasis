# ADR 0056: Preserve right Shift without an E0 prefix

Generation 29 normalizes right Shift (scan code 0x36) to non-extended input
before the source's held-key tracking. Key down, repeat, key up and system-key
messages use the same physical identity even when an IME changes the reported
extended flag. Injected-event filtering and left/right key separation remain.

The Windows injection adapter also strips E0 for 0x36 from older senders.
Right Ctrl, right Alt and other extended scan codes are unchanged, including
the E0 0x2A sequence used with Print Screen. Both PCs should be updated so the
source tracks presses and releases consistently; receiver-only correction does
not repair an older sender's inconsistent held-key identities.

Microsoft documents right Shift as a non-extended 0x36 key:
https://learn.microsoft.com/en-us/windows/win32/inputdev/about-keyboard-input#extended-key-flag

GLFW's Windows backend independently records CJK IME extended-bit behavior:
https://github.com/glfw/glfw/blob/master/src/win32_window.c
These are platform references, not copied implementation code.

Mocked mapping/injection regression tests reproduce incorrect E0 injection and
cover both edges and unrelated extended keys. Actual right Shift plus punctuation
on the paired notebooks remains the hardware acceptance check. No protocol schema,
authorization, clipboard, or database changes are required.
