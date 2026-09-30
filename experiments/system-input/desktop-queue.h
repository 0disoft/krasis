#pragma once
#include <windows.h>

namespace system_trial {
struct QueuedEscape {
    HWND target = nullptr;
    bool (*allowed)(HWND) = nullptr;
    bool queueReady = false;
    bool windowCreated = false;
    bool messageHandled = false;
    bool attempted = false;
    UINT downAccepted = 0;
    UINT upAccepted = 0;
    UINT releaseAccepted = 0;
    DWORD error = 0;
    DWORD code = 33;
};
inline constexpr UINT trialMessage = WM_APP + 0x102;
inline LRESULT CALLBACK trialWindow(HWND window, UINT message, WPARAM wparam, LPARAM lparam) {
    if (message == WM_NCCREATE) {
        auto create = reinterpret_cast<CREATESTRUCTW*>(lparam);
        SetLastError(ERROR_SUCCESS);
        if (!SetWindowLongPtrW(window, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(create->lpCreateParams)) &&
            GetLastError() != ERROR_SUCCESS) return FALSE;
    }
    auto trial = reinterpret_cast<QueuedEscape*>(GetWindowLongPtrW(window, GWLP_USERDATA));
    if (message == trialMessage && trial) {
        // Consume only once, including denial. No duplicate message can replay a key.
        if (trial->messageHandled) return 0;
        trial->messageHandled = true;
        trial->code = 3;
        if (!trial->allowed || !trial->allowed(trial->target)) return 0;
        trial->attempted = true;
        INPUT event{};
        event.type = INPUT_KEYBOARD;
        event.ki.wVk = VK_ESCAPE;
        event.ki.wScan = 0x01;
        SetLastError(ERROR_SUCCESS);
        trial->downAccepted = SendInput(1, &event, sizeof(event));
        if (trial->downAccepted != 1) trial->error = GetLastError();
        // Always release after the down attempt, even if its outcome is uncertain.
        event.ki.dwFlags = KEYEVENTF_KEYUP;
        SetLastError(ERROR_SUCCESS);
        trial->upAccepted = SendInput(1, &event, sizeof(event));
        if (trial->downAccepted == 1 && trial->upAccepted != 1) trial->error = GetLastError();
        trial->code = trial->downAccepted == 1 && trial->upAccepted == 1 ? 0 : 4;
        if (trial->upAccepted != 1) trial->releaseAccepted = SendInput(1, &event, sizeof(event));
        return 0;
    }
    return DefWindowProcW(window, message, wparam, lparam);
}
inline QueuedEscape sendQueuedEscape(HWND target, bool (*allowed)(HWND)) {
    QueuedEscape trial;
    trial.target = target;
    trial.allowed = allowed;
    MSG message{};
    // Only call this after SetThreadDesktop, before creating any worker windows.
    PeekMessageW(&message, nullptr, 0, 0, PM_NOREMOVE);
    trial.queueReady = true;
    HINSTANCE instance = GetModuleHandleW(nullptr);
    constexpr wchar_t className[] = L"KrasisBoundedInputTrial";
    WNDCLASSEXW definition{};
    definition.cbSize = sizeof(definition);
    definition.lpfnWndProc = trialWindow;
    definition.hInstance = instance;
    definition.lpszClassName = className;
    ATOM registered = RegisterClassExW(&definition);
    if (!registered) { trial.error = GetLastError(); return trial; }
    // Never shown or activated. No foreground, capture, focus or broker mutation.
    HWND window = CreateWindowExW(WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW, className, L"", WS_POPUP,
        0, 0, 1, 1, nullptr, nullptr, instance, &trial);
    if (!window) {
        trial.error = GetLastError(); trial.code = 34;
        UnregisterClassW(className, instance); return trial;
    }
    trial.windowCreated = true;
    if (!PostMessageW(window, trialMessage, 0, 0)) {
        trial.code = 35; trial.error = GetLastError();
    } else {
        const ULONGLONG deadline = GetTickCount64() + 500;
        trial.code = 36;
        while (!trial.messageHandled && GetTickCount64() < deadline) {
            if (PeekMessageW(&message, window, trialMessage, trialMessage, PM_REMOVE)) {
                DispatchMessageW(&message);
            } else {
                Sleep(1);
            }
        }
    }
    if (!DestroyWindow(window)) { trial.code = 37; trial.error = GetLastError(); }
    if (!UnregisterClassW(className, instance)) { trial.code = 37; trial.error = GetLastError(); }
    return trial;
}
}
