#pragma once
#include <windows.h>
#include <shlobj.h>
#include <wtsapi32.h>
#include <sddl.h>
#include <string>
#include <cstdio>

namespace system_trial {
inline constexpr wchar_t serviceName[] = L"KrasisSystemInputTrial";
inline std::wstring directory() {
    PWSTR root = nullptr;
    if (FAILED(SHGetKnownFolderPath(FOLDERID_ProgramFiles, KF_FLAG_DEFAULT, nullptr, &root))) return {};
    std::wstring value = std::wstring(root) + L"\\KrasisSystemInputTrial";
    CoTaskMemFree(root);
    return value;
}
inline std::wstring executable() { return directory() + L"\\KrasisSystemInputTrial.exe"; }
inline bool fixedLocation() {
    wchar_t module[32768]{};
    DWORD length = GetModuleFileNameW(nullptr, module, 32768);
    const auto expected = executable();
    return length && length < 32768 && !directory().empty() && _wcsicmp(module, expected.c_str()) == 0;
}
inline bool systemToken(HANDLE token) {
    BYTE data[512]{};
    DWORD size = 0;
    bool ok = GetTokenInformation(token, TokenUser, data, sizeof(data), &size) != 0;
    if (ok) ok = IsWellKnownSid(reinterpret_cast<TOKEN_USER*>(data)->User.Sid, WinLocalSystemSid) != 0;
    return ok;
}
inline bool isSystem() {
    HANDLE token = nullptr;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) return false;
    const bool ok = systemToken(token);
    CloseHandle(token);
    return ok;
}
inline bool tokenState(bool& uiAccess, bool& sessionMatches) {
    HANDLE token = nullptr;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) return false;
    DWORD enabled = 0, session = 0, processSession = 0, size = 0;
    bool ok = GetTokenInformation(token, TokenUIAccess, &enabled, sizeof(enabled), &size) &&
        GetTokenInformation(token, TokenSessionId, &session, sizeof(session), &size) &&
        ProcessIdToSessionId(GetCurrentProcessId(), &processSession);
    uiAccess = ok && enabled == 1;
    sessionMatches = ok && session == processSession;
    CloseHandle(token);
    return ok;
}
inline bool consoleMatchesConfig() {
    auto configuration = directory() + L"\\operator.ini";
    DWORD active = WTSGetActiveConsoleSessionId();
    if (!active || active == 0xFFFFFFFF ||
        GetPrivateProfileIntW(L"trial", L"session", 0, configuration.c_str()) != active) return false;
    wchar_t expected[256]{};
    if (!GetPrivateProfileStringW(L"trial", L"operatorSid", L"", expected, 256, configuration.c_str())) return false;
    HANDLE token = nullptr;
    if (!WTSQueryUserToken(active, &token)) return false;
    BYTE data[512]{}; DWORD size = 0; LPWSTR sid = nullptr;
    bool ok = GetTokenInformation(token, TokenUser, data, sizeof(data), &size) &&
        ConvertSidToStringSidW(reinterpret_cast<TOKEN_USER*>(data)->User.Sid, &sid);
    if (ok) ok = wcscmp(sid, expected) == 0;
    if (sid) LocalFree(sid);
    CloseHandle(token);
    return ok;
}
inline bool currentConsoleSession() {
    DWORD session = 0;
    DWORD active = WTSGetActiveConsoleSessionId();
    return active != 0xFFFFFFFF && active != 0 &&
        ProcessIdToSessionId(GetCurrentProcessId(), &session) && session == active && consoleMatchesConfig();
}
inline bool writeEvidence(const wchar_t* name, DWORD code, DWORD error,
                         bool bound = false, bool already = false, bool attempted = false,
                         UINT accepted = 0, UINT released = 0, bool winlogonToken = false) {
    const auto root = directory();
    if (root.empty()) return false;
    const auto file = root + L"\\" + name;
    HANDLE output = CreateFileW(file.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_NEW,
        FILE_ATTRIBUTE_NORMAL | FILE_FLAG_OPEN_REPARSE_POINT, nullptr);
    if (output == INVALID_HANDLE_VALUE) return false;
    bool uiAccess = false, sessionMatches = false;
    const bool tokenInspected = tokenState(uiAccess, sessionMatches);
    char data[768]{};
    int length = sprintf_s(data,
        "{\"schema\":\"krasis.system-input-trial/v1\",\"code\":%lu,\"win32Error\":%lu,"
        "\"system\":%s,\"consoleSession\":%s,\"desktopBound\":%s,\"alreadyDefault\":%s,"
        "\"inputAttempted\":%s,\"accepted\":%u,\"releaseAccepted\":%u,"
        "\"tokenInspected\":%s,\"uiAccess\":%s,\"tokenSessionMatches\":%s,\"winlogonTokenLaunched\":%s}\n",
        code, error, isSystem() ? "true" : "false", currentConsoleSession() ? "true" : "false",
        bound ? "true" : "false", already ? "true" : "false", attempted ? "true" : "false", accepted, released,
        tokenInspected ? "true" : "false", uiAccess ? "true" : "false", sessionMatches ? "true" : "false",
        winlogonToken ? "true" : "false");
    DWORD written = 0;
    bool ok = length > 0 && WriteFile(output, data, static_cast<DWORD>(length), &written, nullptr) &&
        written == static_cast<DWORD>(length) && FlushFileBuffers(output);
    CloseHandle(output);
    return ok;
}
}
