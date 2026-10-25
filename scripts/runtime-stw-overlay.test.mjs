import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import test from 'node:test';
import { createHash } from 'node:crypto';
import { applyRuntimePatch, prepareRuntimeOverlay, runtimeOverlayEnvironment } from './runtime-stw-overlay.mjs';

const digest = source => createHash('sha256').update(source).digest('hex');
function fixture(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'krasis-stw-'));
  t.after(() => fs.rmSync(directory, { recursive: true }));
  const goroot = path.join(directory, 'Go source');
  fs.mkdirSync(path.join(goroot, 'src/runtime'), { recursive: true });
  const source = 'package runtime\n// before\n';
  const patched = 'package runtime\n// after\n';
  const patch = '@@ -1,2 +1,2 @@\n package runtime\n-// before\n+// after\n';
  const files = ['proc.go', 'proc_test.go', 'proc_runtime_test.go'].map(name => {
    fs.writeFileSync(path.join(goroot, 'src/runtime', name), source);
    return { path: `src/runtime/${name}`, test: name.endsWith('_test.go'), before: digest(source), after: digest(patched), patch };
  });
  return { goroot, version: 'go1.27.3', cacheRoot: path.join(directory, 'cache with spaces'), manifest: { toolchain: 'go1.27.3', files } };
}

test('build replaces only proc.go and leaves the installed toolchain untouched', t => {
  const options = fixture(t);
  const overlay = prepareRuntimeOverlay(options);
  const { Replace } = JSON.parse(fs.readFileSync(overlay, 'utf8'));
  assert.equal(Object.keys(Replace).length, 1);
  for (const [original, replacement] of Object.entries(Replace)) {
    assert.equal(path.basename(original), 'proc.go');
    assert.equal(fs.readFileSync(original, 'utf8'), 'package runtime\n// before\n');
    assert.equal(fs.readFileSync(replacement, 'utf8'), 'package runtime\n// after\n');
  }
  assert.equal(prepareRuntimeOverlay(options), overlay);
});

test('controlled baseline includes regression fixtures without the runtime fix', t => {
  const options = fixture(t);
  const baseline = prepareRuntimeOverlay({ ...options, includeTests: true, baseline: true });
  assert.equal(Object.keys(JSON.parse(fs.readFileSync(baseline, 'utf8')).Replace).length, 2);
  const fixed = prepareRuntimeOverlay({ ...options, includeTests: true });
  assert.equal(Object.keys(JSON.parse(fs.readFileSync(fixed, 'utf8')).Replace).length, 3);
  assert.throws(() => prepareRuntimeOverlay({ ...options, baseline: true }), /configuration/u);
});

test('an unreviewed toolchain or changed original source fails before use', t => {
  const options = fixture(t);
  assert.throws(() => prepareRuntimeOverlay({ ...options, version: 'go1.27.4' }), /Revalidate/u);
  fs.appendFileSync(path.join(options.goroot, 'src/runtime/proc.go'), '// drift\n');
  assert.throws(() => prepareRuntimeOverlay(options), /Unrecognized Go source/u);
});

test('a changed cached source or overlay mapping cannot be reused', t => {
  const options = fixture(t);
  const overlay = prepareRuntimeOverlay(options);
  fs.writeFileSync(overlay, '{}');
  assert.throws(() => prepareRuntimeOverlay(options), /cache differs/u);
  const second = fixture(t);
  const secondOverlay = prepareRuntimeOverlay(second);
  const replacement = Object.values(JSON.parse(fs.readFileSync(secondOverlay, 'utf8')).Replace)[0];
  fs.writeFileSync(replacement, '// damaged\n');
  assert.throws(() => prepareRuntimeOverlay(second), /cache differs/u);
});

test('wrong output digest and ambiguous or missing patch context are rejected', t => {
  const options = fixture(t);
  options.manifest.files[0].after = '0'.repeat(64);
  assert.throws(() => prepareRuntimeOverlay(options), /digest mismatch/u);
  const patch = '@@ -1 +1 @@\n-before\n+after\n';
  assert.throws(() => applyRuntimePatch('other\n', patch), /context mismatch/u);
  assert.throws(() => applyRuntimePatch('before\nbefore\n', patch), /context mismatch/u);
  assert.throws(() => applyRuntimePatch('before\n', ''), /no hunks/u);
});

test('an overlay supplied through GOFLAGS cannot replace the reviewed runtime fix', () => {
  for (const GOFLAGS of ['-overlay=other.json', '"-overlay=other path.json"', '-race -overlay other.json']) {
    assert.throws(() => runtimeOverlayEnvironment({}, { GOFLAGS }), /Existing Go overlay/u);
  }
});
