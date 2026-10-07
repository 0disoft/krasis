import test from 'node:test';
import assert from 'node:assert/strict';
import { createTrayLocaleSync } from './tray-locale.js';

test('tray language changes serialize and end with the latest selection', async () => {
  const calls = [], failures = [];
  let finish;
  const sync = createTrayLocaleSync(locale => {
    calls.push(locale);
    return calls.length === 1 ? new Promise(resolve => { finish = resolve; }) : Promise.resolve(true);
  }, failed => failures.push(failed));
  const initial = sync('ko');
  await sync('en'); await sync('ko'); await sync('en');
  assert.deepEqual(calls, ['ko']);
  finish(true); await initial;
  assert.deepEqual(calls, ['ko', 'en']);
  assert.deepEqual(failures, [false]);
});

test('unsupported language never reaches native code and errors remain visible', async () => {
  const calls = [], failures = [];
  const sync = createTrayLocaleSync(async locale => { calls.push(locale); throw Error('private'); }, failed => failures.push(failed));
  await sync('ko / en'); await sync('constructor'); await sync('ko');
  assert.deepEqual(calls, ['ko']);
  assert.deepEqual(failures, [true, true, true]);
});
