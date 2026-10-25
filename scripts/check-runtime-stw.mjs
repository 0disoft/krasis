import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { offlineGo } from './offline-go.mjs';
import { prepareRuntimeOverlay } from './runtime-stw-overlay.mjs';

if (process.argv.length !== 2) throw Error('Run from the checkout without arguments.');
const go = offlineGo();
const root = process.cwd();
const directory = path.join(root, '.cache/runtime-stw');
const evidence = { toolchain: 'go1.27.3', baseline: null, fixed: null, regression: null };
for (const baseline of [true, false]) {
  const overlay = prepareRuntimeOverlay({ goroot: go.env.GOROOT, version: evidence.toolchain, includeTests: true, baseline });
  const env = { ...go.env, GOMODCACHE: path.join(root, '.cache/go-mod'), GOFLAGS: `"-overlay=${overlay.replaceAll('\\', '/')}"` };
  const result = spawnSync(go.command, ['test', 'runtime', '-run', '^TestStopTheWorldRetakesSyscallP$', '-count', baseline ? '1' : '5', '-timeout=45s', '-v'], {
    env, encoding: 'utf8', timeout: 240000, windowsHide: true,
  });
  const output = (result.stdout ?? '') + (result.stderr ?? '');
  console.log(output);
  if (result.error || baseline && (result.status !== 1 || !/it waited for sysmon/u.test(output)) || !baseline && (result.status !== 0 || /--- SKIP/u.test(output))) {
    throw Error(`Unexpected ${baseline ? 'baseline' : 'fixed'} regression result: ${result.error ?? result.status}`);
  }
  evidence[baseline ? 'baseline' : 'fixed'] = { exitCode: result.status, durations: [...output.matchAll(/stop-the-world duration: ([^\r\n]+)/gu)].map(match => match[1]) };
  if (!baseline) {
    const regression = spawnSync(go.command, ['test', 'runtime', '-run', '^(TestStopTheWorldDeadlock|TestGCFairness2?|TestPreemptionAfterSyscall|TestSyscallN|TestSyscallStackUsage)$', '-count=1', '-timeout=90s', '-v'], {
      env, encoding: 'utf8', timeout: 240000, windowsHide: true,
    });
    console.log((regression.stdout ?? '') + (regression.stderr ?? ''));
    if (regression.error || regression.status !== 0) throw Error('Patched Go runtime regression failed.');
    evidence.regression = { exitCode: regression.status, output: regression.stdout };
  }
}
fs.writeFileSync(path.join(directory, 'probe-results.json'), JSON.stringify(evidence, null, 2) + '\n');
console.log('Confirmed: stock runtime reproduces the stall; the backport passes the controlled regression.');
