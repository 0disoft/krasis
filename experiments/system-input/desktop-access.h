#pragma once
#include <windows.h>

namespace system_trial {
inline constexpr DWORD previousDesktopAccessMask =
    DESKTOP_READOBJECTS | DESKTOP_WRITEOBJECTS | DESKTOP_CREATEWINDOW | DESKTOP_HOOKCONTROL;
// Compare exactly one source-backed difference from Deskflow's GENERIC_WRITE.
// This grants access on our handle; it changes no desktop ACL or system policy.
inline constexpr DWORD inputDesktopAccessMask = previousDesktopAccessMask | DESKTOP_JOURNALPLAYBACK;
static_assert((inputDesktopAccessMask ^ previousDesktopAccessMask) == DESKTOP_JOURNALPLAYBACK);
static_assert((inputDesktopAccessMask & DESKTOP_SWITCHDESKTOP) == 0);
inline bool desktopAccessSmoke() {
    HDESK desktop = OpenInputDesktop(0, FALSE, inputDesktopAccessMask);
    if (!desktop) return false;
    USEROBJECTFLAGS flags{};
    DWORD size = 0;
    const bool ok = GetUserObjectInformationW(desktop, UOI_FLAGS, &flags, sizeof(flags), &size) && !flags.fInherit;
    const bool closed = CloseDesktop(desktop) != 0;
    return ok && closed;
}
}
