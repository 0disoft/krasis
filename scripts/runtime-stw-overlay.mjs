import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sha256 = text => createHash('sha256').update(text).digest('hex');

export function applyRuntimePatch(source, patch) {
  source = source.replaceAll('\r\n', '\n');
  const hunks = patch.split(/^@@ .*@@.*\n/gmu).slice(1);
  if (!hunks.length) throw Error('Runtime patch has no hunks.');
  for (const hunk of hunks) {
    const lines = hunk.split('\n').filter(line => /^[ +\-]/u.test(line));
    const before = lines.filter(line => line[0] !== '+').map(line => line.slice(1)).join('\n');
    const after = lines.filter(line => line[0] !== '-').map(line => line.slice(1)).join('\n');
    const at = source.indexOf(before);
    if (!before || at < 0 || source.indexOf(before, at + 1) >= 0) throw Error('Runtime patch context mismatch.');
    source = source.slice(0, at) + after + source.slice(at + before.length);
  }
  return source;
}

function writeVerified(file, content) {
  try { fs.writeFileSync(file, content, { flag: 'wx' }); }
  catch (error) { if (error.code !== 'EEXIST') throw error; }
  if (fs.readFileSync(file, 'utf8') !== content) throw Error(`Runtime overlay cache differs: ${file}`);
}

// Temporary backport: no installed Go source is changed, and no patch is fetched
// at build time. Unknown toolchains and changed source/cache contents fail closed.
export function prepareRuntimeOverlay({ goroot, version, cacheRoot = path.join(root, '.cache/runtime-stw'), includeTests = false, baseline = false,
  manifest = JSON.parse(fs.readFileSync(path.join(root, 'scripts/runtime/stw-retake.json'), 'utf8')) }) {
  if (version !== manifest.toolchain) throw Error(`Revalidate the temporary STW backport for ${version}; expected ${manifest.toolchain}.`);
  if (!path.isAbsolute(goroot) || baseline && !includeTests) throw Error('Invalid runtime overlay configuration.');
  const key = sha256(goroot + JSON.stringify(manifest));
  const directory = path.join(cacheRoot, key, baseline ? 'baseline' : includeTests ? 'test' : 'build');
  fs.mkdirSync(directory, { recursive: true });
  const Replace = {};
  for (const file of manifest.files) {
    if (file.test && !includeTests || !file.test && baseline) continue;
    if (!/^src\/runtime\/proc(?:_runtime_test|_test)?\.go$/u.test(file.path)) throw Error('Unexpected runtime overlay path.');
    const original = path.join(goroot, file.path);
    const source = fs.readFileSync(original, 'utf8').replaceAll('\r\n', '\n');
    if (sha256(source) !== file.before) throw Error(`Unrecognized Go source: ${file.path}`);
    const patched = applyRuntimePatch(source, file.patch);
    if (sha256(patched) !== file.after) throw Error(`Runtime patch digest mismatch: ${file.path}`);
    const replacement = path.join(directory, path.basename(file.path));
    writeVerified(replacement, patched);
    Replace[original] = replacement;
  }
  const overlay = path.join(directory, 'overlay.json');
  writeVerified(overlay, JSON.stringify({ Replace }, null, 2) + '\n');
  return overlay;
}

export function runtimeOverlayEnvironment(selected, environment) {
  if (/(?:^|\s)["']?-overlay(?:=|\s|$)/u.test(environment.GOFLAGS ?? '')) throw Error('Existing Go overlay would bypass the STW backport.');
  const overlay = prepareRuntimeOverlay({ goroot: selected.GOROOT, version: selected.GOVERSION });
  return { ...environment, GOFLAGS: `${environment.GOFLAGS ?? ''} "-overlay=${overlay.replaceAll('\\', '/')}"`.trim() };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const selected = JSON.parse(execFileSync('go', ['env', '-json', 'GOROOT', 'GOVERSION'], { encoding: 'utf8', windowsHide: true }));
  console.log(JSON.stringify({ GOFLAGS: runtimeOverlayEnvironment(selected, process.env).GOFLAGS }));
}
