#include <windows.h>

#include <winrt/Windows.UI.Input.Preview.Injection.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/base.h>

#include <cwchar>
#include <iterator>
#include <vector>

using namespace winrt::Windows::UI::Input::Preview::Injection;

namespace {

enum class target_kind { credential_broker, elevated_shell };

bool default_input_desktop() {
    HDESK desktop = OpenInputDesktop(0, FALSE, DESKTOP_READOBJECTS);
    if (!desktop) return false;
    wchar_t name[128]{};
    DWORD needed = 0;
    const BOOL read = GetUserObjectInformationW(desktop, UOI_NAME, name, sizeof(name), &needed);
    CloseDesktop(desktop);
    return read && wcscmp(name, L"Default") == 0;
}

bool high_integrity(HANDLE process) {
    HANDLE token = nullptr;
    if (!OpenProcessToken(process, TOKEN_QUERY, &token)) return false;
    DWORD needed = 0;
    GetTokenInformation(token, TokenIntegrityLevel, nullptr, 0, &needed);
    std::vector<BYTE> data(needed);
    const BOOL read = needed >= sizeof(TOKEN_MANDATORY_LABEL) &&
        GetTokenInformation(token, TokenIntegrityLevel, data.data(), needed, &needed);
    bool high = false;
    if (read) {
        const auto* label = reinterpret_cast<const TOKEN_MANDATORY_LABEL*>(data.data());
        const auto count = *GetSidSubAuthorityCount(label->Label.Sid);
        high = count != 0 &&
            *GetSidSubAuthority(label->Label.Sid, count - 1) >= SECURITY_MANDATORY_HIGH_RID;
    }
    CloseHandle(token);
    return high;
}

bool same_user(HANDLE process) {
    HANDLE self_token = nullptr;
    HANDLE target_token = nullptr;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &self_token)) return false;
    if (!OpenProcessToken(process, TOKEN_QUERY, &target_token)) {
        CloseHandle(self_token);
        return false;
    }
    auto user_sid = [](HANDLE token, std::vector<BYTE>& data) -> PSID {
        DWORD needed = 0;
        GetTokenInformation(token, TokenUser, nullptr, 0, &needed);
        if (needed < sizeof(TOKEN_USER)) return nullptr;
        data.resize(needed);
        if (!GetTokenInformation(token, TokenUser, data.data(), needed, &needed)) return nullptr;
        return reinterpret_cast<TOKEN_USER*>(data.data())->User.Sid;
    };
    std::vector<BYTE> self_data;
    std::vector<BYTE> target_data;
    PSID self = user_sid(self_token, self_data);
    PSID other = user_sid(target_token, target_data);
    const bool same = self && other && EqualSid(self, other);
    CloseHandle(target_token);
    CloseHandle(self_token);
    return same;
}

bool matches_foreground(HWND window, target_kind target) {
    if (!window || !default_input_desktop() || window != GetForegroundWindow()) return false;
    DWORD pid = 0;
    GetWindowThreadProcessId(window, &pid);
    if (!pid) return false;
    DWORD self_session = 0;
    DWORD target_session = 0;
    if (!ProcessIdToSessionId(GetCurrentProcessId(), &self_session) ||
        !ProcessIdToSessionId(pid, &target_session) || self_session != target_session) return false;
    HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if (!process) return false;
    wchar_t image[1024]{};
    DWORD length = static_cast<DWORD>(std::size(image));
    const BOOL identified = QueryFullProcessImageNameW(process, 0, image, &length);
    bool matches = false;
    if (identified) {
        const wchar_t* basename = wcsrchr(image, L'\\');
        basename = basename ? basename + 1 : image;
        if (target == target_kind::credential_broker) {
            matches = _wcsicmp(basename, L"CredentialUIBroker.exe") == 0;
        } else {
            matches = (_wcsicmp(basename, L"pwsh.exe") == 0 ||
                       _wcsicmp(basename, L"powershell.exe") == 0) &&
                      high_integrity(process) && same_user(process);
        }
    }
    CloseHandle(process);
    return matches && window == GetForegroundWindow();
}

void result(const wchar_t* message) {
    MessageBoxW(nullptr, message, L"Krasis 입력 경로 시험", MB_OK | MB_ICONINFORMATION);
}

}  // namespace

int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) {
    const int choice = MessageBoxW(nullptr,
        L"PIN 창은 예, 관리자 PowerShell은 아니요를 누르세요.\n"
        L"이 시험은 네트워크나 Krasis 연결을 사용하지 않습니다.",
        L"Krasis 입력 경로 시험", MB_YESNOCANCEL | MB_ICONQUESTION);
    if (choice == IDCANCEL) return 0;
    const target_kind target = choice == IDYES ? target_kind::credential_broker
                                               : target_kind::elevated_shell;

    try {
        winrt::init_apartment(winrt::apartment_type::multi_threaded);
        auto injector = InputInjector::TryCreate();
        if (!injector) {
            result(L"input_injector_unavailable: Windows 입력 권한을 얻지 못했습니다.");
            return 2;
        }
        result(target == target_kind::credential_broker
            ? L"확인을 누른 뒤 2분 안에 B에서 PIN 창을 띄우세요. PIN은 입력하지 마세요."
            : L"확인을 누른 뒤 2분 안에 B에서 관리자 PowerShell을 전면에 두세요.\n"
              L"빈 명령줄에 시험용 글자를 입력해 두면 Esc 효과를 확인하기 쉽습니다.");

        HWND observed = nullptr;
        const ULONGLONG deadline = GetTickCount64() + 120000;
        while (GetTickCount64() < deadline) {
            HWND window = GetForegroundWindow();
            if (matches_foreground(window, target) && matches_foreground(window, target)) {
                observed = window;
                break;
            }
            Sleep(50);
        }
        if (!observed || !matches_foreground(observed, target)) {
            result(L"target_not_observed: 대상 창을 확인하지 못했습니다.");
            return 3;
        }

        InjectedInputKeyboardInfo down;
        down.VirtualKey(VK_ESCAPE);
        InjectedInputKeyboardInfo up;
        up.VirtualKey(VK_ESCAPE);
        up.KeyOptions(InjectedInputKeyOptions::KeyUp);
        injector.InjectKeyboardInput(std::vector<InjectedInputKeyboardInfo>{down, up});

        if (target == target_kind::elevated_shell) {
            result(L"escape_sent: 관리자 PowerShell의 시험용 글자가 지워졌는지 직접 확인해 주세요.");
            return 0;
        }
        Sleep(1000);
        if (matches_foreground(observed, target)) {
            result(L"dialog_still_open: PIN 창이 그대로 전면에 있습니다.");
            return 4;
        }
        result(L"dialog_dismissed_candidate: PIN 창이 실제로 닫혔는지 직접 확인해 주세요.");
        return 0;
    } catch (const winrt::hresult_error&) {
        result(L"input_api_failed: Windows 입력 API 호출에 실패했습니다.");
        return 5;
    }
}
