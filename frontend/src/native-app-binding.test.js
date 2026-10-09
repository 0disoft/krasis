import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { createBridge } from './bridge.js';

test('manual refresh uses the actual app binding rather than a test-only native mock', async () => {
  const source = readFileSync(new URL('./NativeApp.svelte', import.meta.url), 'utf8');
  const match = source.match(/bridge\s*=\s*createBridge\((\{[\s\S]*?\})\);/);
  assert.ok(match, 'actual workspace binding must be exercised');
  const calls = [];
  const snapshot = { schema: 'krasis.desktop/v1', state: 'LOCAL', peerAlias: '', peers: [], code: 'ok', notice: '' };
  const Call = { ByName: async method => { calls.push(method); return snapshot; } };
  const binding = Function('Call', 'name', 'return (' + match[1] + ')')(Call, 'WorkspaceService.');
  const bridge = createBridge(binding);
  await bridge.refresh();
  const result = await bridge.refreshNow();
  assert.deepEqual(calls, ['WorkspaceService.Refresh', 'WorkspaceService.RefreshNow']);
  assert.equal(result.state, 'LOCAL');
  assert.equal(result.code, 'ok');
});
