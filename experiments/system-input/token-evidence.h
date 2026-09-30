#pragma once
#include <windows.h>
#include <cstdio>
#include <cstring>

namespace system_trial {
struct TokenNumber {
    bool known = false;
    DWORD value = 0;
};
struct TokenEvidence {
    TokenNumber integrityRid, elevated, elevationType, uiAccess, session, restricted, appContainer;
    DWORD queryError = 0;
};
inline void tokenError(TokenEvidence& evidence, DWORD error) {
    if (!evidence.queryError) evidence.queryError = error ? error : ERROR_INVALID_DATA;
}
inline TokenNumber tokenNumber(HANDLE token, TOKEN_INFORMATION_CLASS field, TokenEvidence& evidence) {
    DWORD value = 0, size = 0;
    if (!GetTokenInformation(token, field, &value, sizeof(value), &size)) {
        tokenError(evidence, GetLastError()); return {};
    }
    if (size != sizeof(value)) { tokenError(evidence, ERROR_INVALID_DATA); return {}; }
    return {true, value};
}
inline TokenEvidence inspectToken(HANDLE token) {
    TokenEvidence result;
    // Retain only the mandatory-label RID. Never serialize SID or privilege data.
    alignas(TOKEN_MANDATORY_LABEL) BYTE label[256]{};
    DWORD size = 0;
    if (GetTokenInformation(token, TokenIntegrityLevel, label, sizeof(label), &size)) {
        auto sid = reinterpret_cast<TOKEN_MANDATORY_LABEL*>(label)->Label.Sid;
        if (IsValidSid(sid) && *GetSidSubAuthorityCount(sid)) {
            result.integrityRid = {true, *GetSidSubAuthority(sid, *GetSidSubAuthorityCount(sid) - 1)};
        } else { tokenError(result, ERROR_INVALID_SID); }
    } else { tokenError(result, GetLastError()); }
    result.elevated = tokenNumber(token, TokenElevation, result);
    result.elevationType = tokenNumber(token, TokenElevationType, result);
    result.uiAccess = tokenNumber(token, TokenUIAccess, result);
    result.session = tokenNumber(token, TokenSessionId, result);
    result.appContainer = tokenNumber(token, TokenIsAppContainer, result);
    SetLastError(ERROR_SUCCESS);
    const BOOL restricted = IsTokenRestricted(token);
    const DWORD restrictedError = GetLastError();
    if (restricted || restrictedError == ERROR_SUCCESS) result.restricted = {true, restricted ? 1UL : 0UL};
    else tokenError(result, restrictedError);
    return result;
}
struct InputTokenEvidence {
    bool captured = false;
    const char* effectiveSource = "unknown";
    TokenEvidence process, effective;
};
inline InputTokenEvidence captureInputTokens() {
    InputTokenEvidence result;
    result.captured = true;
    HANDLE token = nullptr;
    if (OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) {
        result.process = inspectToken(token); CloseHandle(token);
    } else { result.process.queryError = GetLastError(); }
    token = nullptr;
    if (OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, TRUE, &token)) {
        result.effectiveSource = "thread";
        result.effective = inspectToken(token); CloseHandle(token);
    } else {
        const DWORD error = GetLastError();
        if (error == ERROR_NO_TOKEN) {
            result.effectiveSource = "process";
            result.effective = result.process;
        } else { result.effective.queryError = error; }
    }
    return result;
}
inline const char* tokenValue(const TokenNumber& value, char (&buffer)[16]) {
    if (!value.known) return "null";
    sprintf_s(buffer, "%lu", value.value); return buffer;
}
inline void tokenJSON(const TokenEvidence& value, char (&buffer)[512]) {
    char rid[16]{}, elevated[16]{}, kind[16]{}, access[16]{}, session[16]{}, restricted[16]{}, container[16]{};
    sprintf_s(buffer,
        "{\"integrityRid\":%s,\"elevated\":%s,\"elevationType\":%s,\"uiAccess\":%s,"
        "\"session\":%s,\"restricted\":%s,\"appContainer\":%s,\"queryError\":%lu}",
        tokenValue(value.integrityRid, rid), tokenValue(value.elevated, elevated),
        tokenValue(value.elevationType, kind), tokenValue(value.uiAccess, access),
        tokenValue(value.session, session), tokenValue(value.restricted, restricted),
        tokenValue(value.appContainer, container), value.queryError);
}
inline bool tokenEvidenceSmoke() {
    const auto captured = captureInputTokens();
    if (!captured.captured || strcmp(captured.effectiveSource, "process") != 0 ||
        captured.process.queryError || !captured.process.integrityRid.known ||
        !captured.process.elevated.known || !captured.process.elevationType.known ||
        !captured.process.uiAccess.known || !captured.process.session.known ||
        !captured.process.restricted.known || !captured.process.appContainer.known) return false;
    const auto denied = inspectToken(nullptr);
    char json[512]{}; tokenJSON(denied, json);
    return denied.queryError == ERROR_INVALID_HANDLE && !denied.integrityRid.known &&
        !denied.elevated.known && !denied.uiAccess.known &&
        strstr(json, "\"integrityRid\":null") && strstr(json, "\"elevated\":null") &&
        strstr(json, "\"uiAccess\":null");
}
}
