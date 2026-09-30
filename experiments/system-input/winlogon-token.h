#pragma once
#include <windows.h>
#include <tlhelp32.h>
#include <cwchar>

namespace system_trial {
inline bool enablePrivilege(const wchar_t* name, DWORD& error) {
    HANDLE token = nullptr;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY, &token)) {
        error = GetLastError(); return false;
    }
    TOKEN_PRIVILEGES privileges{};
    privileges.PrivilegeCount = 1;
    privileges.Privileges[0].Attributes = SE_PRIVILEGE_ENABLED;
    bool ok = LookupPrivilegeValueW(nullptr, name, &privileges.Privileges[0].Luid) != 0;
    if (!ok) error = GetLastError();
    if (ok) {
        SetLastError(ERROR_SUCCESS);
        ok = AdjustTokenPrivileges(token, FALSE, &privileges, 0, nullptr, nullptr) != 0;
        error = GetLastError();
        ok = ok && error == ERROR_SUCCESS;
    }
    CloseHandle(token);
    return ok;
}
inline bool tokenInSession(HANDLE token, DWORD session) {
    DWORD value = 0, size = 0;
    return GetTokenInformation(token, TokenSessionId, &value, sizeof(value), &size) && value == session;
}
inline HANDLE consoleWinlogonToken(DWORD session, DWORD& error) {
    // The source is fixed. Never accept a caller-supplied PID, image or session.
    HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (snapshot == INVALID_HANDLE_VALUE) { error = GetLastError(); return nullptr; }
    PROCESSENTRY32W entry{};
    entry.dwSize = sizeof(entry);
    DWORD candidate = 0;
    bool enumerated = Process32FirstW(snapshot, &entry) != 0;
    if (!enumerated) { error = GetLastError(); CloseHandle(snapshot); return nullptr; }
    do {
        DWORD processSession = 0;
        if (_wcsicmp(entry.szExeFile, L"winlogon.exe") != 0 ||
            !ProcessIdToSessionId(entry.th32ProcessID, &processSession) || processSession != session) continue;
        if (candidate) { error = ERROR_DUP_NAME; CloseHandle(snapshot); return nullptr; }
        candidate = entry.th32ProcessID;
    } while (Process32NextW(snapshot, &entry));
    error = GetLastError();
    CloseHandle(snapshot);
    if (error != ERROR_NO_MORE_FILES) return nullptr;
    if (!candidate) { error = ERROR_NOT_FOUND; return nullptr; }

    HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, candidate);
    if (!process) { error = GetLastError(); return nullptr; }
    wchar_t image[32768]{}, expected[MAX_PATH]{};
    DWORD length = 32768, processSession = 0;
    UINT systemLength = GetSystemDirectoryW(expected, MAX_PATH);
    bool identified = systemLength && systemLength < MAX_PATH &&
        wcscat_s(expected, L"\\winlogon.exe") == 0 &&
        QueryFullProcessImageNameW(process, 0, image, &length) && _wcsicmp(image, expected) == 0 &&
        GetProcessId(process) == candidate && ProcessIdToSessionId(candidate, &processSession) &&
        processSession == session && WTSGetActiveConsoleSessionId() == session;
    HANDLE source = nullptr, token = nullptr;
    if (!identified) { error = ERROR_INVALID_OWNER; CloseHandle(process); return nullptr; }
    if (!OpenProcessToken(process, TOKEN_QUERY | TOKEN_DUPLICATE, &source)) {
        error = GetLastError(); CloseHandle(process); return nullptr;
    }
    if (!systemToken(source) || !tokenInSession(source, session)) {
        error = ERROR_INVALID_OWNER;
    } else if (!DuplicateTokenEx(source, TOKEN_QUERY | TOKEN_DUPLICATE | TOKEN_ASSIGN_PRIMARY |
        TOKEN_ADJUST_DEFAULT, nullptr, SecurityImpersonation, TokenPrimary, &token)) {
        error = GetLastError();
    } else {
        error = ERROR_SUCCESS;
    }
    CloseHandle(source);
    CloseHandle(process);
    return token;
}
}
