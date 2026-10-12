import test from 'node:test';
import assert from 'node:assert/strict';
import { nativeCatalogs, nativeText } from './native-i18n.js';
test('live UI catalogs cover states and bridge errors without raw messages', () => {
  assert.deepEqual(Object.keys(nativeCatalogs.ko).sort(), Object.keys(nativeCatalogs.en).sort());
  for (const name of ['title','network','pair','move','start']) {
    assert.ok(nativeCatalogs.ko['welcome_' + name]); assert.ok(nativeCatalogs.en['welcome_' + name]);
  }
  for (const key of ['CONNECTING','RECONNECTING','DISCONNECTING','UNKNOWN','stale_peer','outcome_unknown','recovery_required',
    'startup_resume_settings_invalid','startup_resume_settings_unavailable','input_setup_unavailable',
    ...['NOT_INSTALLED','UPDATE_REQUIRED','READY','UPDATING','CANCELLED','FAILED','PENDING','UNKNOWN','SERVICE_STOPPED'].map(state => 'input_setup_' + state)]) {
    assert.ok(nativeCatalogs.ko[key]); assert.ok(nativeCatalogs.en[key]);
  }
  assert.equal(nativeText('en', 'private native exception'), nativeCatalogs.en.unavailable);
});
