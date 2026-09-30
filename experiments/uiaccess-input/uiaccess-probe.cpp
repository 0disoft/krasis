#define WIN32_LEAN_AND_MEAN
#include <windows.h>

#include <cwchar>
#include <iterator>

namespace {

bool uiAccessEnabled() {
    HANDLE token = nullptr;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) return false;
    DWORD enabled = 0;
    DWORD size = 0;
    const bool ok = GetTokenInformation(token, TokenUIAccess, &enabled, sizeof(enabled), &size) && enabled == 1;
    CloseHandle(token);
    return ok;
}

bool isDefaultDesktop(HDESK desktop) {
    if (!desktop) return false;
    wchar_t name[128]{};
    DWORD needed = 0;
    return GetUserObjectInformationW(desktop, UOI_NAME, name, sizeof(name), &needed) &&
           wcscmp(name, L"Default") == 0;
}

bool onDefaultDesktop() {
    HDESK desktop = OpenInputDesktop(0, FALSE, DESKTOP_READOBJECTS);
    const bool ok = isDefaultDesktop(desktop);
    if (desktop) CloseDesktop(desktop);
    return ok;
}

bool credentialBrokerForeground(HWND window) {
    if (!window || window != GetForegroundWindow() || !onDefaultDesktop()) return false;
    DWORD pid = 0;
    GetWindowThreadProcessId(window, &pid);
    if (!pid) return false;
    DWORD currentSession = 0;
    DWORD targetSession = 0;
    if (!ProcessIdToSessionId(GetCurrentProcessId(), &currentSession) ||
        !ProcessIdToSessionId(pid, &targetSession) || currentSession != targetSession) return false;
    HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if (!process) return false;
    wchar_t image[1024]{};
    DWORD length = static_cast<DWORD>(std::size(image));
    const bool identified = QueryFullProcessImageNameW(process, 0, image, &length) != 0;
    CloseHandle(process);
    if (!identified) return false;
    wchar_t systemDirectory[MAX_PATH]{};
    const UINT systemLength = GetSystemDirectoryW(systemDirectory, static_cast<UINT>(std::size(systemDirectory)));
    if (!systemLength || systemLength >= std::size(systemDirectory) ||
        wcscat_s(systemDirectory, L"\\CredentialUIBroker.exe") != 0) return false;
    return _wcsicmp(image, systemDirectory) == 0 && window == GetForegroundWindow();
}

void report(const wchar_t* message) {
#ifdef KRASIS_SYSTEM_TRIAL
    (void)message; // The privileged child never displays a dialog.
#else
    MessageBoxW(nullptr, message, L"Krasis UIAccess 입력 시험", MB_OK | MB_ICONINFORMATION);
#endif
}

struct EscapeResult {
    UINT accepted = 0;
    DWORD error = ERROR_SUCCESS;
    UINT releaseAccepted = 0;
};

#ifndef KRASIS_SYSTEM_TRIAL
EscapeResult sendEscape() {
    INPUT events[2]{};
    for (auto& event : events) {
        event.type = INPUT_KEYBOARD;
        event.ki.wVk = VK_ESCAPE;
        event.ki.wScan = 0x01;
        event.ki.dwFlags = 0;
        event.ki.dwExtraInfo = 0;
    }
    events[1].ki.dwFlags |= KEYEVENTF_KEYUP;
    EscapeResult result;
    SetLastError(ERROR_SUCCESS);
    result.accepted = SendInput(2, events, sizeof(INPUT));
    // Capture the primary call's error before cleanup can overwrite it.
    if (result.accepted == 2) return result;
    result.error = GetLastError();
    // A partial batch must not leave Escape pressed.
    result.releaseAccepted = SendInput(1, &events[1], sizeof(INPUT));
    return result;
}
#endif

struct DesktopTrial {
    HWND target;
    bool alreadyDefault = false;
    bool bound = false;
    bool inputAttempted = false;
    EscapeResult escape;
#ifdef KRASIS_SYSTEM_TRIAL
    bool queueReady = false;
    bool hiddenWindowCreated = false;
    bool messageHandled = false;
    UINT downAccepted = 0;
    UINT upAccepted = 0;
#endif
};

DWORD WINAPI inputDesktopTrial(void* parameter) {
    auto& trial = *static_cast<DesktopTrial*>(parameter);
    // Bind before creating any windows/hooks; destroy the transient queue window
    // before restoring the original desktop. Never rebind the UI thread.
    HDESK original = GetThreadDesktop(GetCurrentThreadId());
    trial.alreadyDefault = isDefaultDesktop(original);
    HDESK desktop = OpenInputDesktop(0, FALSE,
        DESKTOP_READOBJECTS | DESKTOP_WRITEOBJECTS | DESKTOP_CREATEWINDOW | DESKTOP_HOOKCONTROL);
    if (!desktop) return 6;
    DWORD result = 7;
    if (isDefaultDesktop(desktop)) {
        result = 8;
        if (SetThreadDesktop(desktop)) {
            trial.bound = true;
            result = 3;
            // Recheck identity, foreground and active desktop after binding.
            if (credentialBrokerForeground(trial.target)
#ifdef KRASIS_SYSTEM_TRIAL
                && system_trial::currentConsoleSession()
#endif
            ) {
                bool complete = false;
#ifdef KRASIS_SYSTEM_TRIAL
                const auto queued = system_trial::sendQueuedEscape(trial.target, [](HWND target) {
                    return credentialBrokerForeground(target) && system_trial::currentConsoleSession();
                });
                trial.queueReady = queued.queueReady;
                trial.hiddenWindowCreated = queued.windowCreated;
                trial.messageHandled = queued.messageHandled;
                trial.downAccepted = queued.downAccepted;
                trial.upAccepted = queued.upAccepted;
                trial.inputAttempted = queued.attempted;
                trial.escape = {queued.downAccepted + queued.upAccepted, queued.error, queued.releaseAccepted};
                result = queued.code;
                complete = queued.code == 0 && trial.escape.accepted == 2;
#else
                result = 4;
                trial.inputAttempted = true;
                trial.escape = sendEscape();
                complete = trial.escape.accepted == 2;
#endif
                if (complete) {
                    Sleep(1000);
                    result = IsWindow(trial.target) && IsWindowVisible(trial.target) ? 5 : 0;
                }
            }
            // Leave the opened desktop before closing it. If restoration fails,
            // thread exit releases the attachment; the process owns the handle.
            if (!SetThreadDesktop(original)) return 10;
        }
    }
    if (!CloseDesktop(desktop)) return 10;
    return result;
}

void reportTrial(DWORD result, const DesktopTrial& trial) {
#ifdef KRASIS_SYSTEM_TRIAL
    system_trial::writeEvidence(L"result.json", result, trial.escape.error, trial.bound,
        trial.alreadyDefault, trial.inputAttempted, trial.escape.accepted, trial.escape.releaseAccepted,
        false, trial.queueReady, trial.hiddenWindowCreated, trial.messageHandled, trial.downAccepted, trial.upAccepted);
#else
    const wchar_t* message = L"input_thread_failed: 입력 시험 스레드를 실행하지 못했습니다.";
    switch (result) {
    case 0: message = L"dialog_dismissed_candidate: PIN 창이 실제로 닫혔는지 B에서 확인하세요."; break;
    case 3: message = L"target_not_observed: 대상 PIN 창이 바뀌어 입력을 보내지 않았습니다."; break;
    case 4: message = L"escape_send_failed: Esc 입력 전송에 실패했습니다."; break;
    case 5: message = L"dialog_still_open: 입력 데스크톱 연결 후에도 PIN 창이 열려 있습니다."; break;
    case 6: message = L"input_desktop_open_failed: 입력 데스크톱을 열지 못해 입력을 보내지 않았습니다."; break;
    case 7: message = L"input_desktop_excluded: 일반 사용자 데스크톱이 아니므로 입력을 보내지 않았습니다."; break;
    case 8: message = L"input_desktop_bind_failed: 입력 데스크톱에 연결하지 못해 입력을 보내지 않았습니다."; break;
    case 10: message = L"input_desktop_cleanup_failed: 데스크톱 복구에 실패했습니다. 시험 프로그램을 종료합니다."; break;
    }
    wchar_t details[768]{};
    swprintf_s(details, L"%s\n\n입력 데스크톱 연결: %s\n시험 스레드가 원래도 같은 데스크톱: %s\nEsc 전송 시도: %s\nEsc 입력 수락: %u / 2\nWin32 오류 번호: %lu\n실패 후 키 뗌 수락: %u / 1\n\n오류 번호만으로 Windows의 차단 원인을 확정할 수 없습니다.\n결과를 확인한 뒤 B 터치패드로 이 창을 닫으세요.",
        message, trial.bound ? L"성공" : L"실패", trial.alreadyDefault ? L"예" : L"아니요",
        trial.inputAttempted ? L"예" : L"아니요", trial.escape.accepted, trial.escape.error,
        trial.escape.releaseAccepted);
    report(details);
#endif
}

}  // namespace

int runInputTrial() {
    if (!uiAccessEnabled()) {
        report(L"uiaccess_unavailable: 서명과 설치 위치를 확인하세요. 시험은 입력을 보내지 않았습니다.");
        return 2;
    }
    report(L"확인을 누른 뒤 2분 안에 B에서 Chrome의 Windows 보안 PIN 창을 띄우세요.\nPIN은 입력하지 마세요. 이 시험은 Esc 한 번만 보냅니다.");
    const ULONGLONG deadline = GetTickCount64() + 120000;
    HWND observed = nullptr;
    while (GetTickCount64() < deadline) {
        HWND window = GetForegroundWindow();
        if (credentialBrokerForeground(window)) {
            observed = window;
            break;
        }
        Sleep(50);
    }
    if (!observed || !credentialBrokerForeground(observed)) {
        report(L"target_not_observed: 대상 PIN 창을 확인하지 못해 입력을 보내지 않았습니다.");
        return 3;
    }
    DesktopTrial trial{observed};
    HANDLE worker = CreateThread(nullptr, 0, inputDesktopTrial, &trial, 0, nullptr);
    if (!worker) {
        report(L"input_thread_failed: 입력 시험 스레드를 만들지 못해 입력을 보내지 않았습니다.");
        return 9;
    }
    // No timed-out worker may outlive its target/context or send delayed input.
    if (WaitForSingleObject(worker, 5000) != WAIT_OBJECT_0) {
        ExitProcess(9);
    }
    DWORD result = 9;
    GetExitCodeThread(worker, &result);
    CloseHandle(worker);
    reportTrial(result, trial);
    return static_cast<int>(result);
}

#ifndef KRASIS_SYSTEM_TRIAL
int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) { return runInputTrial(); }
#endif
