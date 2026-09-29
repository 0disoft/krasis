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

bool onDefaultDesktop() {
    HDESK desktop = OpenInputDesktop(0, FALSE, DESKTOP_READOBJECTS);
    if (!desktop) return false;
    wchar_t name[128]{};
    DWORD needed = 0;
    const bool ok = GetUserObjectInformationW(desktop, UOI_NAME, name, sizeof(name), &needed) &&
                    wcscmp(name, L"Default") == 0;
    CloseDesktop(desktop);
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
    MessageBoxW(nullptr, message, L"Krasis UIAccess 입력 시험", MB_OK | MB_ICONINFORMATION);
}

bool sendEscape() {
    INPUT events[2]{};
    for (auto& event : events) {
        event.type = INPUT_KEYBOARD;
        event.ki.wScan = 0x01;
        event.ki.dwFlags = KEYEVENTF_SCANCODE;
        event.ki.dwExtraInfo = 0x4B524153;
    }
    events[1].ki.dwFlags |= KEYEVENTF_KEYUP;
    const UINT sent = SendInput(2, events, sizeof(INPUT));
    if (sent == 2) return true;
    // A partial batch must not leave Escape pressed.
    SendInput(1, &events[1], sizeof(INPUT));
    return false;
}

}  // namespace

int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) {
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
    if (!sendEscape()) {
        report(L"escape_send_failed: Esc 입력 전송에 실패했습니다.");
        return 4;
    }
    Sleep(1000);
    if (credentialBrokerForeground(observed)) {
        report(L"dialog_still_open: Esc 전송 후에도 PIN 창이 전면에 있습니다.");
        return 5;
    }
    report(L"dialog_dismissed_candidate: PIN 창이 실제로 닫혔는지 B에서 확인하세요.");
    return 0;
}
