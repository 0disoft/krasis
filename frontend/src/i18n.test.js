import test from 'node:test';
import assert from 'node:assert/strict';
import { messages, resolveLocale, translate } from './i18n.js';

test('catalogs have identical keys and placeholders', () => {
  assert.deepEqual(Object.keys(messages.ko).sort(), Object.keys(messages.en).sort());
  for (const key of Object.keys(messages.en)) {
    assert.deepEqual(messages.ko[key].match(/\{\w+\}/g), messages.en[key].match(/\{\w+\}/g));
  }
});
test('explicit choice takes precedence and locale matching is bounded', () => {
  assert.equal(resolveLocale('en', ['ko-KR']), 'en');
  assert.equal(resolveLocale(null, ['ko-KR']), 'ko');
  assert.equal(resolveLocale('constructor', ['fr-FR']), 'en');
  assert.equal(resolveLocale(null, ['de-DE', 'en-US']), 'en');
});
test('interpolation and missing keys fail visibly', () => {
  assert.equal(translate('ko', 'rearmValue', { value: 32 }), '32px');
  assert.throws(() => translate('ko', 'missing'));
  assert.throws(() => translate('en', 'rearmValue'));
});
