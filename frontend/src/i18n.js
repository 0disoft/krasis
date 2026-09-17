export const messages = {
  ko: {
    workspace: '작업 공간', devices: '기기', settings: '설정', preview: '디자인 미리보기', previewHelp: '실제 기기 연결이나 설정은 변경되지 않습니다.',
    placement: '기기 배치', placementHelp: '책상 위 순서대로 화면을 배치하세요.', connected: '연결됨', local: '연결 대기', faulted: '복구 필요',
    connect: '연결', disconnect: '연결 해제', retry: '에이전트 재시작', thisPC: '이 PC', otherPC: '다른 PC', left: '왼쪽', right: '오른쪽',
    placementLocked: '배치 변경은 연결 해제 후 가능합니다.', placementReady: '상대 기기의 위치를 선택하세요.', addDevice: '기기 추가', swap: '좌우 배치 바꾸기',
    clipboard: '클립보드 공유', clipboardHelp: '두 PC 사이에서 텍스트를 공유합니다.', update: '기기 업데이트', updateHelp: '상대 기기의 버전을 확인하고 업데이트합니다.',
    unverified: '버전 확인 전', diagnostics: '진단 내보내기', diagnosticsHelp: '문제 해결을 위한 진단 정보를 저장합니다.',
    permission: '등록한 두 기기 사이에서만 키보드·마우스 제어를 허용합니다.', language: '언어', close: '닫기',
    pairingTitle: '새 기기 등록', pairingHelp: '한쪽 PC에서 PIN을 만들고, 다른 PC에서 주변 기기를 찾아 입력합니다.',
    pairingPreview: 'PIN 생성과 기기 검색은 다음 단계에서 기존 에이전트에 연결합니다.', createPIN: 'PIN 만들기', findDevice: '주변 기기 찾기',
    deviceHelp: '등록한 기기와 공유 권한을 관리합니다.', paired: '등록된 기기', trusted: '페어링된 기기',
    settingsHelp: '언어와 화면 전환 설정을 관리합니다.', rearm: '재진입 거리', rearmHelp: '화면을 넘어온 뒤 다시 전환하기 전 이동해야 하는 거리입니다.',
    rearmValue: '{value}px', automatic: '자동 복구', automaticHelp: '오류 진단을 먼저 저장하고, 안전한 재시작을 최대 3회 시도합니다.',
    faultHelp: '입력 제어를 중지했습니다. 진단을 저장한 뒤 안전하게 복구합니다.',
    previewAction: '미리보기입니다. 실제 작업은 실행되지 않았습니다.', scenario: '표시 상태', connectedScenario: '연결됨', localScenario: '연결 대기', faultScenario: '오류',
    version: '앱 정보', versionHelp: '관리 앱·에이전트 버전은 실제 연결 후 표시합니다.', notConnected: '에이전트 미연결',
  },
  en: {
    workspace: 'Workspace', devices: 'Devices', settings: 'Settings', preview: 'Design preview', previewHelp: 'No real device connections or settings are changed.',
    placement: 'Device arrangement', placementHelp: 'Arrange your screens to match your desk.', connected: 'Connected', local: 'Ready to connect', faulted: 'Recovery needed',
    connect: 'Connect', disconnect: 'Disconnect', retry: 'Restart agent', thisPC: 'This PC', otherPC: 'Other PC', left: 'Left', right: 'Right',
    placementLocked: 'Disconnect before changing the arrangement.', placementReady: 'Choose where the other device is placed.', addDevice: 'Add device', swap: 'Swap left and right',
    clipboard: 'Clipboard sharing', clipboardHelp: 'Share text between your two PCs.', update: 'Device update', updateHelp: 'Check and update the other device.',
    unverified: 'Not checked', diagnostics: 'Export diagnostics', diagnosticsHelp: 'Save diagnostic information for troubleshooting.',
    permission: 'Keyboard and mouse control is allowed only between your paired devices.', language: 'Language', close: 'Close',
    pairingTitle: 'Pair a new device', pairingHelp: 'Create a PIN on one PC, then find it and enter the PIN on the other.',
    pairingPreview: 'PIN creation and discovery will be connected to the existing agent in the next stage.', createPIN: 'Create PIN', findDevice: 'Find nearby devices',
    deviceHelp: 'Manage paired devices and sharing permissions.', paired: 'Paired devices', trusted: 'Paired device',
    settingsHelp: 'Manage language and screen transition settings.', rearm: 'Re-entry distance', rearmHelp: 'The distance to move after crossing before another transition is allowed.',
    rearmValue: '{value}px', automatic: 'Automatic recovery', automaticHelp: 'Save diagnostics first, then attempt a safe restart up to three times.',
    faultHelp: 'Input control has stopped. Diagnostics are saved before safe recovery.',
    previewAction: 'This is a preview. No real action was performed.', scenario: 'Display state', connectedScenario: 'Connected', localScenario: 'Ready', faultScenario: 'Faulted',
    version: 'About', versionHelp: 'Control app and agent versions appear after a real connection.', notConnected: 'No agent connection',
  },
};

export function resolveLocale(saved, languages = []) {
  if (Object.hasOwn(messages, saved)) return saved;
  for (const language of languages) {
    const base = language.toLowerCase().split('-')[0];
    if (Object.hasOwn(messages, base)) return base;
  }
  return 'en';
}

export function translate(locale, key, values = {}) {
  const message = messages[locale]?.[key] ?? messages.en[key];
  if (message === undefined) throw new Error(`Unknown message key: ${key}`);
  return message.replace(/\{(\w+)\}/g, (_, name) => {
    if (!Object.hasOwn(values, name)) throw new Error(`Missing placeholder: ${name}`);
    return String(values[name]);
  });
}
