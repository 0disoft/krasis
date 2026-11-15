import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

export const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
export const digestFile = file => sha256(fs.readFileSync(file));
export const within = (root, file) => {
  const relative = path.relative(path.resolve(root), path.resolve(file));
  return relative !== '' && !relative.startsWith('..') && !path.isAbsolute(relative);
};
export function plain(file) {
  let cursor = path.resolve(file);
  for (;;) {
    if (fs.lstatSync(cursor).isSymbolicLink()) throw Error('Linked retention path');
    const parent = path.dirname(cursor);
    if (parent === cursor) break;
    cursor = parent;
  }
}
export function readJSON(file, guards) {
  plain(file);
  if (fs.statSync(file).size > 1024 * 1024) throw Error('Oversized retention record');
  const bytes = fs.readFileSync(file);
  guards?.set(file, sha256(bytes));
  return JSON.parse(bytes.toString('utf8').replace(/^\uFEFF/u, ''));
}
export function assertGuards(guards) {
  for (const [file, hash] of guards) {
    if (hash === null) {
      if (fs.existsSync(file)) throw Error('New retention references appeared');
      continue;
    }
    plain(file);
    if (digestFile(file) !== hash) throw Error('Recovery or retention references changed');
  }
}
export function sourceState(repo) {
  const guards = new Map();
  const file = path.join(repo, 'internal/localapi/runtime.go');
  plain(file);
  const bytes = fs.readFileSync(file);
  const generation = Number(bytes.toString('utf8').match(/^const AgentGeneration = ([1-9][0-9]*)\r?$/mu)?.[1]);
  if (!Number.isSafeInteger(generation)) throw Error('Unknown source generation');
  guards.set(file, sha256(bytes));
  return {generations: [generation], revisions: [], paths: [], inputHashes: [], guards};
}

// Local paths/launch bytes remain in memory; no identity, trust or control data is read.
export function windowsRuntimeState(repo, controllerPID = process.pid) {
  const state = {generations: [], revisions: [], paths: [], inputHashes: [], guards: new Map()};
  if (process.platform !== 'win32') return state;
  if (!process.env.LOCALAPPDATA || !process.env.ProgramFiles) throw Error('Missing Windows product roots');
  const script = fileURLToPath(new URL('./read-artifact-retention-state.ps1', import.meta.url));
  const processes = JSON.parse(execFileSync('pwsh.exe', ['-NoProfile', '-NonInteractive', '-File', script, '-ControllerPID', String(controllerPID)], {
    encoding: 'utf8', windowsHide: true, timeout: 30_000, maxBuffer: 1024 * 1024,
  }));
  if (processes.busy) throw Error('Another Krasis build or installer is active');
  state.paths.push(...processes.paths);
  const local = path.join(process.env.LOCALAPPDATA, 'Krasis');
  const launch = path.join(local, 'launch');
  if (fs.existsSync(path.join(local, 'app/installation.json')) && !fs.existsSync(path.join(launch, 'current.json'))) throw Error('Missing current launch record');
  const bind = (directory, record) => {
    const file = path.join(directory, 'BUILD.json');
    const build = readJSON(file, state.guards);
    if (!Number.isSafeInteger(build.generation) || build.generation < 1 || !/^[a-f0-9]{40}$/u.test(build.revision) || build.platform !== 'windows/amd64') throw Error('Unknown live build');
    if (record && (record.schema !== 'krasis.launch/v1' || record.generation !== build.generation || record.revision !== build.revision || !/^[a-f0-9]{64}$/u.test(record.sha256))) throw Error('Invalid recovery launch record');
    if (record) {
      plain(record.executable);
      if (path.basename(record.executable).toLowerCase() !== 'krasis.exe' || digestFile(record.executable) !== record.sha256) throw Error('Recovery executable differs');
      state.guards.set(record.executable, record.sha256);
    }
    state.generations.push(build.generation);
    state.revisions.push(build.revision);
  };
  if (fs.existsSync(launch)) {
    plain(launch);
    for (const entry of fs.readdirSync(launch, {withFileTypes: true})) {
      if (!entry.name.endsWith('.json')) continue;
      const record = readJSON(path.join(launch, entry.name), state.guards);
      if (['current.json', 'previous.json', 'recovery-task.json'].includes(entry.name)) {
        if (typeof record.executable !== 'string' || !path.isAbsolute(record.executable)) throw Error('Unknown recovery executable');
        bind(path.dirname(record.executable), record);
      }
    }
  }
  for (const file of processes.paths) {
    if (path.isAbsolute(file) && within(path.join(local, 'app'), file)) bind(path.dirname(file));
  }
  const input = path.join(process.env.ProgramFiles, 'Krasis/Input');
  if (fs.existsSync(input)) {
    for (const name of ['krasis-agent.exe', 'krasis-input.exe']) {
      const file = path.join(input, name);
      plain(file);
      const hash = digestFile(file);
      state.guards.set(file, hash);
      state.inputHashes.push(hash);
    }
  }
  // Snapshot contents must remain stable until planning finishes.
  assertGuards(state.guards);
  return state;
}
