import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';

// Local manual-test artifact only: never starts an agent or sends input.
const root = process.cwd();
const git = args => execFileSync('git', args, { encoding: 'utf8' }).trim();
if (git(['status', '--porcelain'])) throw new Error('Build the committed, clean source tree.');
const revision = git(['rev-parse', 'HEAD']);
const destination = path.join(root, 'dist', 'product-smoke', revision);
if (fs.existsSync(destination)) throw new Error('Artifact already exists; do not overwrite it.');
fs.mkdirSync(destination, { recursive: true });
const environment = { ...process.env, GOOS: 'windows', GOARCH: 'amd64', CGO_ENABLED: '0', GOPROXY: 'off', GOSUMDB: 'off', GOCACHE: path.join(root, '.cache', 'go-build') };
for (const name of ['krasis', 'krasis-agent']) {
  const executable = path.join(destination, `${name}.exe`);
  execFileSync('go', ['build', '-trimpath', '-o', executable, `./cmd/${name}`], { env: environment, stdio: 'inherit' });
  const bytes = fs.readFileSync(executable);
  const pe = bytes.readUInt32LE(0x3c);
  if (bytes.toString('ascii', 0, 2) !== 'MZ' || bytes.readUInt32LE(pe) !== 0x4550 || bytes.readUInt16LE(pe + 4) !== 0x8664) {
    throw new Error('Artifact is not a Windows x64 executable.');
  }
}
const probe = spawnSync(path.join(destination, 'krasis.exe'), [], { encoding: 'utf8', timeout: 5000 });
if (probe.error || probe.status !== 2) throw new Error('CLI usage smoke check failed.');
fs.copyFileSync('docs/experiments/product-two-pc-smoke.md', path.join(destination, 'START-HERE.md'));
fs.writeFileSync(path.join(destination, 'BUILD.json'), JSON.stringify({ revision, platform: 'windows/amd64', cgo: false, purpose: 'manual-two-pc-smoke', signed: false }, null, 2) + '\n');
const entries = ['krasis.exe', 'krasis-agent.exe', 'START-HERE.md', 'BUILD.json'];
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
fs.writeFileSync(path.join(destination, 'SHA256SUMS.txt'), entries.map(name => `${hash(path.join(destination, name))}  ${name}`).join('\n') + '\n');
entries.push('SHA256SUMS.txt');
const archive = path.join(destination, 'krasis-windows-x64-smoke.zip');
const quote = value => "'" + value.replaceAll("'", "''") + "'";
execFileSync('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', `$ErrorActionPreference='Stop'; Compress-Archive -LiteralPath @(${entries.map(name => quote(path.join(destination, name))).join(',')}) -DestinationPath ${quote(archive)}`], { stdio: 'inherit' });
process.stdout.write(JSON.stringify({ archive, sha256: hash(archive), revision, entries }, null, 2) + '\n');
