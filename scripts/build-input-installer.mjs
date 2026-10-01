import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
import { verifyExecutableIcon } from './check-windows-icon.mjs';

// Build only: no elevation, SCM changes, installer execution or certificates.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
assert.equal(path.resolve('.'), root, 'Run from the Krasis checkout.');
assert.equal(process.argv.length, 2, 'This builder takes no paths or arguments.');
const git = args => execFileSync('git', args, { encoding: 'utf8' }).trim();
assert.equal(git(['status', '--porcelain']), '', 'Commit source changes before building embedded payloads.');
const revision = git(['rev-parse', 'HEAD']);
assert.match(revision, /^[a-f0-9]{40}$/u);
const output = path.join(root, 'dist/input-installer', revision);
const command = path.join(root, 'cmd/krasis-install');
const payload = path.join(command, 'payload');
const resource = path.join(command, 'installer_windows_amd64.syso');
assert.ok(!fs.existsSync(output), 'Never overwrite an existing installer artifact.');
assert.ok(!fs.existsSync(resource), 'Unexpected existing generated resource.');
fs.mkdirSync(output, { recursive: true });
fs.mkdirSync(payload, { recursive: true });
const env = {
  ...process.env, GOOS: 'windows', GOARCH: 'amd64', CGO_ENABLED: '0',
  GOPROXY: 'off', GOSUMDB: 'off', GOFLAGS: '',
  GOMODCACHE: path.join(root, '.cache/go-mod'), GOCACHE: path.join(root, '.cache/go-build'),
};
const build = (name, destination, tags = []) => execFileSync('go', [
  'build', '-trimpath', '-buildvcs=true', ...tags, '-ldflags=-s -w',
  '-o', destination, `./cmd/${name}`,
], { env, stdio: 'inherit' });
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
build('krasis-agent', path.join(payload, 'krasis-agent.exe'));
build('krasis-input', path.join(payload, 'krasis-input.exe'));
const pins = {
  agent: hash(fs.readFileSync(path.join(payload, 'krasis-agent.exe'))),
  input: hash(fs.readFileSync(path.join(payload, 'krasis-input.exe'))),
};
fs.writeFileSync(path.join(payload, 'pins.json'), JSON.stringify(pins));
const tools = path.join(root, '.cache/tools/llvm-mingw-20260908-ucrt-x86_64/bin');
const res = path.join(output, 'installer.res');
execFileSync(path.join(tools, 'llvm-rc.exe'), ['/FO', res, 'krasis-install.rc'], {
  cwd: path.join(root, 'assets/windows'), stdio: 'inherit',
});
try {
  execFileSync(path.join(tools, 'llvm-cvtres.exe'), [
    '/MACHINE:X64', '/TIMESTAMP:0', `/OUT:${resource}`, res,
  ], { stdio: 'inherit' });
  const executable = path.join(output, 'krasis-install.exe');
  build('krasis-install', executable, ['-tags', 'inputinstaller']);
  const bytes = fs.readFileSync(executable);
  verifyExecutableIcon(bytes);
  assert.deepEqual(readManifest(bytes), fs.readFileSync(path.join(root, 'assets/windows/krasis-install.manifest')));
  assert.equal(git(['status', '--porcelain']), '', 'Builder changed source files.');
  fs.writeFileSync(path.join(output, 'BUILD.json'), JSON.stringify({
    schemaVersion: 'krasis.input.installer-build/v1', revision, platform: 'windows/amd64',
    sha256: hash(bytes), agentSha256: pins.agent, inputSha256: pins.input,
    executionLevel: 'requireAdministrator', signed: false, installed: false,
  }, null, 2) + '\n', { flag: 'wx' });
  process.stdout.write(`Input installer built; not installed: ${output}\n`);
} finally {
  // A requireAdministrator resource must not leak into ordinary go test builds.
  if (fs.existsSync(resource)) fs.unlinkSync(resource);
}

function readManifest(bytes) {
  const pe = bytes.readUInt32LE(0x3c), optional = pe + 24;
  assert.equal(bytes.readUInt32LE(pe), 0x4550);
  assert.equal(bytes.readUInt16LE(optional), 0x20b);
  const sections = optional + bytes.readUInt16LE(pe + 20);
  const offset = rva => {
    for (let i = 0; i < bytes.readUInt16LE(pe + 6); i++) {
      const at = sections + i * 40, start = bytes.readUInt32LE(at + 12);
      if (rva >= start && rva - start < bytes.readUInt32LE(at + 16)) {
        return bytes.readUInt32LE(at + 20) + rva - start;
      }
    }
    throw Error('Resource RVA is outside the executable.');
  };
  const base = offset(bytes.readUInt32LE(optional + 128));
  const child = (directory, id) => {
    const count = bytes.readUInt16LE(directory + 12) + bytes.readUInt16LE(directory + 14);
    for (let i = 0; i < count; i++) {
      const at = directory + 16 + 8 * i;
      if (id === undefined || bytes.readUInt32LE(at) === id) return bytes.readUInt32LE(at + 4);
    }
    throw Error('Missing installer manifest resource.');
  };
  const directory = value => {
    assert.ok(value & 0x80000000);
    return base + (value & 0x7fffffff);
  };
  const leaf = child(directory(child(directory(child(base, 24)), 1)));
  assert.equal(leaf & 0x80000000, 0);
  const entry = base + leaf, start = offset(bytes.readUInt32LE(entry)), length = bytes.readUInt32LE(entry + 4);
  assert.ok(start + length <= bytes.length);
  return bytes.subarray(start, start + length);
}
