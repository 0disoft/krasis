#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include "trial-common.h"
#include "winlogon-token.h"
#define KRASIS_SYSTEM_TRIAL
#include "../uiaccess-input/uiaccess-probe.cpp"

namespace {
SERVICE_STATUS_HANDLE statusHandle = nullptr;
HANDLE stopEvent = nullptr;

void status(DWORD state, DWORD code = 0) {
    SERVICE_STATUS value{};
    value.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    value.dwCurrentState = state;
    value.dwControlsAccepted = state == SERVICE_RUNNING ? SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN : 0;
    value.dwWin32ExitCode = code ? ERROR_SERVICE_SPECIFIC_ERROR : NO_ERROR;
    value.dwServiceSpecificExitCode = code;
    if (state == SERVICE_START_PENDING) { value.dwCheckPoint = 1; value.dwWaitHint = 10000; }
    SetServiceStatus(statusHandle, &value);
}
DWORD WINAPI control(DWORD event, DWORD, void*, void*) {
    if ((event == SERVICE_CONTROL_STOP || event == SERVICE_CONTROL_SHUTDOWN) && stopEvent) SetEvent(stopEvent);
    return NO_ERROR;
}
DWORD launchTrial(DWORD& error, bool& winlogonTokenLaunched) {
    DWORD session = WTSGetActiveConsoleSessionId();
    if (!session || session == 0xFFFFFFFF || !system_trial::isSystem() || !system_trial::fixedLocation()) return 20;
    if (!system_trial::enablePrivilege(SE_TCB_NAME, error) ||
        !system_trial::enablePrivilege(SE_DEBUG_NAME, error) || !system_trial::consoleMatchesConfig()) return 22;
    auto marker = system_trial::directory() + L"\\run-claimed";
    HANDLE claim = CreateFileW(marker.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_NEW,
        FILE_ATTRIBUTE_NORMAL | FILE_FLAG_OPEN_REPARSE_POINT, nullptr);
    if (claim == INVALID_HANDLE_VALUE) { error = GetLastError(); return 21; }
    CloseHandle(claim);

    HANDLE token = system_trial::consoleWinlogonToken(session, error);
    if (!token) return 32;
    DWORD enabled = 1;
    BOOL prepared = SetTokenInformation(token, TokenUIAccess, &enabled, sizeof(enabled));
    if (!prepared) { error = GetLastError(); CloseHandle(token); return 23; }

    HANDLE job = CreateJobObjectW(nullptr, nullptr);
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits{};
    limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    if (!job || !SetInformationJobObject(job, JobObjectExtendedLimitInformation, &limits, sizeof(limits))) {
        error = GetLastError(); if (job) CloseHandle(job); CloseHandle(token); return 24;
    }
    STARTUPINFOEXW startup{};
    startup.StartupInfo.cb = sizeof(startup);
    SIZE_T attributeSize = 0;
    InitializeProcThreadAttributeList(nullptr, 1, 0, &attributeSize);
    startup.lpAttributeList = static_cast<LPPROC_THREAD_ATTRIBUTE_LIST>(HeapAlloc(GetProcessHeap(), 0, attributeSize));
    if (!startup.lpAttributeList) { CloseHandle(job); CloseHandle(token); return 24; }
    BOOL initialized = InitializeProcThreadAttributeList(startup.lpAttributeList, 1, 0, &attributeSize);
    if (!initialized || !UpdateProcThreadAttribute(startup.lpAttributeList, 0, PROC_THREAD_ATTRIBUTE_JOB_LIST,
        &job, sizeof(job), nullptr, nullptr)) {
        error = GetLastError();
        if (initialized) DeleteProcThreadAttributeList(startup.lpAttributeList);
        HeapFree(GetProcessHeap(), 0, startup.lpAttributeList); CloseHandle(job); CloseHandle(token); return 24;
    }
    wchar_t desktop[] = L"winsta0\\default";
    startup.StartupInfo.lpDesktop = desktop;
    PROCESS_INFORMATION child{};
    auto binary = system_trial::executable();
    std::wstring command = L"\"" + binary + L"\" --worker";
    wchar_t environment[] = L"\0";
    auto root = system_trial::directory();
    // Do not create a child after an operator/session change during token preparation.
    if (!system_trial::consoleMatchesConfig() || !system_trial::tokenInSession(token, session)) {
        DeleteProcThreadAttributeList(startup.lpAttributeList);
        HeapFree(GetProcessHeap(), 0, startup.lpAttributeList);
        CloseHandle(job); CloseHandle(token); return 20;
    }
    BOOL created = CreateProcessAsUserW(token, binary.c_str(), command.data(), nullptr, nullptr, FALSE,
        CREATE_SUSPENDED | CREATE_UNICODE_ENVIRONMENT | CREATE_NO_WINDOW | EXTENDED_STARTUPINFO_PRESENT,
        environment, root.c_str(), &startup.StartupInfo, &child);
    error = created ? ERROR_SUCCESS : GetLastError();
    DeleteProcThreadAttributeList(startup.lpAttributeList);
    HeapFree(GetProcessHeap(), 0, startup.lpAttributeList);
    CloseHandle(token);
    if (!created) { CloseHandle(job); return 25; }
    winlogonTokenLaunched = true;
    DWORD result = 26;
    // The child belongs to the kill-on-close job at creation, even if we crash
    // before resuming it. There is no unowned suspended-process interval.
    if (ResumeThread(child.hThread) != static_cast<DWORD>(-1)) {
        const ULONGLONG deadline = GetTickCount64() + 130000;
        HANDLE waits[] = {child.hProcess, stopEvent};
        while (GetTickCount64() < deadline && system_trial::consoleMatchesConfig()) {
            DWORD waited = WaitForMultipleObjects(2, waits, FALSE, 100);
            if (waited == WAIT_OBJECT_0) { GetExitCodeProcess(child.hProcess, &result); break; }
            if (waited == WAIT_OBJECT_0 + 1) { result = 27; break; }
            if (waited == WAIT_FAILED) { error = GetLastError(); result = 28; break; }
            result = 29;
        }
    } else { error = GetLastError(); }
    // Only terminate the exact child we created, including failed job assignment.
    if (WaitForSingleObject(child.hProcess, 0) != WAIT_OBJECT_0) TerminateProcess(child.hProcess, result);
    CloseHandle(job);
    if (WaitForSingleObject(child.hProcess, 5000) != WAIT_OBJECT_0) result = 30;
    CloseHandle(child.hThread);
    CloseHandle(child.hProcess);
    return result;
}
void WINAPI serviceMain(DWORD argc, wchar_t**) {
    statusHandle = RegisterServiceCtrlHandlerExW(system_trial::serviceName, control, nullptr);
    if (!statusHandle) return;
    status(SERVICE_START_PENDING);
    stopEvent = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    if (!stopEvent || argc != 1) { status(SERVICE_STOPPED, 20); return; }
    status(SERVICE_RUNNING);
    DWORD error = 0;
    bool winlogonTokenLaunched = false;
    DWORD result = launchTrial(error, winlogonTokenLaunched);
    system_trial::writeEvidence(L"service-result.json", result, error, false, false, false, 0, 0,
        winlogonTokenLaunched);
    status(SERVICE_STOPPED, result);
    // Keep the handler's event valid until this one-shot process exits.
}
}

int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR command, int) {
    if (!system_trial::fixedLocation() || !system_trial::isSystem()) return 12;
    if (wcscmp(command, L"--worker") == 0) {
        DWORD error = 0;
        if (!system_trial::enablePrivilege(SE_TCB_NAME, error)) {
            system_trial::writeEvidence(L"result.json", 22, error); return 22;
        }
        if (!system_trial::currentConsoleSession()) return 12;
        bool uiAccess = false, sessionMatches = false;
        if (!system_trial::tokenState(uiAccess, sessionMatches) || !uiAccess || !sessionMatches) {
            system_trial::writeEvidence(L"result.json", 23, ERROR_INVALID_OWNER); return 23;
        }
        int result = runInputTrial();
        // A detailed result already exists after injection; CREATE_NEW preserves it.
        system_trial::writeEvidence(L"result.json", static_cast<DWORD>(result), 0);
        return result;
    }
    if (wcscmp(command, L"--service") != 0) return 12;
    SERVICE_TABLE_ENTRYW table[] = {{const_cast<wchar_t*>(system_trial::serviceName), serviceMain}, {nullptr, nullptr}};
    return StartServiceCtrlDispatcherW(table) ? 0 : 31;
}
