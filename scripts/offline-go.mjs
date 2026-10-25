import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { runtimeOverlayEnvironment } from './runtime-stw-overlay.mjs';

// Select and verify through Go's ordinary launcher before disabling network
// resolution. A downloaded toolchain still needs checksum verification during
// selection, even when all its files already exist in the module cache.
export function offlineGo(environment = process.env) {
  // Keep the selected compiler outside the dependency cache: Go refuses
  // overlays of any file beneath GOMODCACHE, including a downloaded GOROOT.
  const selectionEnvironment = { ...environment };
  delete selectionEnvironment.GOMODCACHE;
  const selected = JSON.parse(execFileSync('go', ['env', '-json', 'GOROOT', 'GOVERSION'], {
    env: selectionEnvironment, encoding: 'utf8', windowsHide: true,
  }));
  const module = fs.readFileSync('go.mod', 'utf8');
  const required = module.match(/^toolchain (go\d+\.\d+\.\d+)\s*$/mu)?.[1];
  if (!required) throw Error('Offline builds require the repository toolchain baseline.');
  const version = value => {
    const match = /^go(\d+)\.(\d+)\.(\d+)$/u.exec(value);
    if (!match) throw Error('Offline builds require a stable Go release.');
    return match.slice(1).map(Number);
  };
  const actual = version(selected.GOVERSION), minimum = version(required);
  const difference = actual.map((n, i) => n - minimum[i]).find(n => n !== 0) ?? 0;
  if (difference < 0) throw Error('Selected Go is older than the repository toolchain baseline.');
  if (!path.isAbsolute(selected.GOROOT)) throw Error('Selected Go root must be absolute.');
  const command = path.join(selected.GOROOT, 'bin', process.platform === 'win32' ? 'go.exe' : 'go');
  // Invoke that compiler directly. "local" now refers to the verified
  // selected compiler, not an older system launcher on PATH.
  const env = { ...environment, GOMODCACHE: environment.GOMODCACHE || path.resolve('.cache/go-mod'), GOROOT: selected.GOROOT, GOTOOLCHAIN: 'local', GOPROXY: 'off', GOSUMDB: 'off' };
  const relative = path.relative(env.GOMODCACHE, selected.GOROOT);
  if (!relative.startsWith('..' + path.sep) && relative !== '..' && !path.isAbsolute(relative)) {
    throw Error('The runtime overlay requires a Go installation outside GOMODCACHE.');
  }
  if (execFileSync(command, ['env', 'GOVERSION'], { env, encoding: 'utf8', windowsHide: true }).trim() !== selected.GOVERSION) {
    throw Error('Selected Go root does not contain the selected compiler.');
  }
  return { command, env: runtimeOverlayEnvironment(selected, env) };
}
