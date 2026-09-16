# ADR 0049: Physical press ownership at grant admission

Physical key/button routing and source activation share a short ownership gate.
A held physical press prevents grant activation, including keys held before hook
installation (checked through the native pressed-state query). Repeat downs are
idempotent; a local down and its matching up therefore stay local during REQUEST.

Mouse buttons now have one synchronous publisher: the low-level mouse hook.
Raw Input still supplies pointer motion, but delayed raw button packets cannot
replay a pre-grant press into a later lease. Injected keyboard/mouse events are
excluded. This supersedes Raw Input button ownership in earlier source design.

Tests cover keyboard/button local pairs across grant admission, repeats, native
held-state denial, injected-button filtering, and delayed raw-button rejection.
Two-PC tests remain necessary for touchpad gestures and device-specific buttons.
