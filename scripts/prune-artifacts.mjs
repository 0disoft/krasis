import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {plain, readJSON, digestFile, sha256, within, assertGuards, sourceState, windowsRuntimeState} from './artifact-retention-state.mjs';

const day = 24 * 60 * 60 * 1000;
const categories = ['workspace', 'app-setup', 'input-installer'];
const bundleNames = ['BUILD.json', 'SHA256SUMS.txt', 'START-HERE.md', 'THIRD-PARTY-NOTICES.txt', 'krasis.exe', 'krasis-agent.exe', 'krasis-cli.exe', 'krasis-install.exe'];
const revisionPattern = /^[a-f0-9]{40}$/u;
const hashPattern = /^[a-f0-9]{64}$/u;
const repository = fileURLToPath(new URL('../', import.meta.url));

function inspect(directory, category) {
  plain(directory);
  const entries = fs.readdirSync(directory, {withFileTypes: true});
  for (const entry of entries) plain(path.join(directory, entry.name));
  const record = {directory, category, valid: false, reason: 'unrecognized-content', action: 'Keep', files: [], bytes: 0};
  if (!entries.some(e => e.name === 'BUILD.json' && e.isFile())) return record;
  const meta = readJSON(path.join(directory, 'BUILD.json'));
  record.revision = meta.revision;
  record.generation = meta.generation;
  record.meta = meta;
  if (!revisionPattern.test(meta.revision)) return record;
  let names;
  if (category === 'workspace') {
    if (meta.generation !== Number(path.basename(directory).slice(1)) || meta.platform !== 'windows/amd64') return record;
    names = bundleNames;
  } else if (category === 'app-setup') {
    if (meta.schema !== 'krasis.setup-build/v1' || meta.generation !== Number(path.basename(directory).slice(1)) || !hashPattern.test(meta.sha256)) return record;
    names = ['BUILD.json', 'SHA256SUMS.txt', 'setup.res', 'KrasisSetup-g' + meta.generation + '.exe'];
  } else {
    if (meta.schemaVersion !== 'krasis.input.installer-build/v1' || meta.revision !== path.basename(directory) || meta.platform !== 'windows/amd64' ||
        meta.executionLevel !== 'requireAdministrator' || meta.installed !== false || ![meta.sha256, meta.agentSha256, meta.inputSha256].every(h => hashPattern.test(h))) return record;
    names = ['BUILD.json', 'installer.res', 'krasis-install.exe'];
  }
  if (entries.length !== names.length || entries.some(e => !e.isFile() || !names.includes(e.name))) return record;
  const stats = [fs.statSync(directory)];
  record.files = names.map(name => {
    const file = path.join(directory, name), stat = fs.statSync(file);
    stats.push(stat);
    return {name, bytes: stat.size, modified: stat.mtimeMs, created: stat.birthtimeMs, sha256: digestFile(file)};
  });
  record.bytes = record.files.reduce((n, f) => n + f.bytes, 0);
  record.newest = Math.max(...stats.flatMap(s => [s.mtimeMs, s.birthtimeMs]));
  const hashes = new Map(record.files.map(f => [f.name, f.sha256]));
  if (category !== 'input-installer') {
    const lines = fs.readFileSync(path.join(directory, 'SHA256SUMS.txt'), 'utf8').trim().split(/\r?\n/u);
    const expected = category === 'workspace' ? names.filter(n => n !== 'SHA256SUMS.txt') : ['KrasisSetup-g' + meta.generation + '.exe'];
    const seen = new Set();
    if (lines.length !== expected.length) return record;
    for (const line of lines) {
      const match = /^([a-f0-9]{64})  ([a-zA-Z0-9_.-]+)$/u.exec(line);
      if (!match || !expected.includes(match[2]) || seen.has(match[2]) || hashes.get(match[2]) !== match[1]) return record;
      seen.add(match[2]);
    }
  }
  if (category === 'app-setup' && hashes.get('KrasisSetup-g' + meta.generation + '.exe') !== meta.sha256) return record;
  if (category === 'input-installer' && hashes.get('krasis-install.exe') !== meta.sha256) return record;
  record.valid = true;
  record.reason = null;
  return record;
}

function protection(repo, runtimeReader, keepPaths) {
  const source = sourceState(repo), runtime = runtimeReader(repo);
  const state = {
    generations: new Set([528, 535, 536, ...source.generations, ...runtime.generations]),
    revisions: new Set(runtime.revisions), paths: [...runtime.paths, ...keepPaths].map(p => path.resolve(repo, p)),
    inputHashes: runtime.inputHashes, guards: new Map([...source.guards, ...runtime.guards]),
  };
  const file = path.join(repo, 'dist/artifact-retention-pins.json');
  state.guards.set(file, null);
  if (fs.existsSync(file)) {
    const pins = readJSON(file, state.guards);
    if (pins.schema !== 'krasis.artifact-retention-pins/v1' || Object.keys(pins).some(k => !['schema', 'generations', 'revisions', 'paths'].includes(k))) throw Error('Unknown retention pins');
    for (const g of pins.generations ?? []) {
      if (!Number.isSafeInteger(g) || g < 1) throw Error('Invalid pinned generation');
      state.generations.add(g);
    }
    for (const r of pins.revisions ?? []) {
      if (!revisionPattern.test(r)) throw Error('Invalid pinned revision');
      state.revisions.add(r);
    }
    for (const p of pins.paths ?? []) {
      if (typeof p !== 'string' || !within(path.join(repo, 'dist'), path.resolve(repo, p))) throw Error('Invalid pinned artifact path');
      state.paths.push(path.resolve(repo, p));
    }
  }
  const reviews = path.join(repo, 'dist/review');
  if (fs.existsSync(reviews)) {
    plain(reviews);
    for (const revision of fs.readdirSync(reviews, {withFileTypes: true})) {
      if (!revision.isDirectory() || !revisionPattern.test(revision.name)) continue;
      const directory = path.join(reviews, revision.name); plain(directory);
      state.revisions.add(revision.name);
      for (const bundle of fs.readdirSync(directory, {withFileTypes: true})) {
        if (!bundle.isDirectory() || !/^g[1-9][0-9]*$/u.test(bundle.name)) continue;
        const file = path.join(directory, bundle.name, 'BUILD.json');
        plain(path.dirname(file));
        if (!fs.existsSync(file)) { state.guards.set(file, null); continue; }
        const metadata = readJSON(file, state.guards);
        if (metadata.revision !== revision.name || metadata.generation !== Number(bundle.name.slice(1))) throw Error('Unknown retained review build');
        state.revisions.add(metadata.revision);
      }
    }
  }
  return state;
}

function pinReason(record, state, inputRevisions) {
  if (state.generations.has(record.generation)) return 'protected-generation';
  if (state.revisions.has(record.revision) || inputRevisions.has(record.revision)) return 'live-or-recovery-revision';
  if (state.paths.some(p => p === record.directory || within(record.directory, p) || within(p, record.directory))) return 'active-or-explicit-path';
  return null;
}
function installedInputRevisions(records, state) {
  const matching = records.filter(r => r.valid && r.category === 'input-installer' &&
    state.inputHashes.includes(r.meta.agentSha256) && state.inputHashes.includes(r.meta.inputSha256));
  if (state.inputHashes.length && !matching.length) throw Error('Cannot identify the installed input recovery installer');
  return new Set(matching.map(r => r.revision));
}

export function planArtifactRetention({repoRoot = repository, now = Date.now(), keepPaths = [], runtimeReader = windowsRuntimeState} = {}) {
  const repo = path.resolve(repoRoot);
  plain(repo);
  const state = protection(repo, runtimeReader, keepPaths), records = [];
  for (const category of categories) {
    const root = path.join(repo, 'dist', category);
    if (!fs.existsSync(root)) continue;
    plain(root);
    for (const entry of fs.readdirSync(root, {withFileTypes: true})) {
      plain(path.join(root, entry.name));
      if (!entry.isDirectory() || !(category === 'input-installer' ? revisionPattern.test(entry.name) : /^g[1-9][0-9]*$/u.test(entry.name))) continue;
      records.push(inspect(path.join(root, entry.name), category));
    }
  }
  const inputRevisions = installedInputRevisions(records, state);
  for (const r of records) {
    if (!r.valid) continue;
    r.reason = pinReason(r, state, inputRevisions) ?? (r.newest >= now - 2 * day ? 'younger-than-48-hours' : null);
  }
  for (const category of categories) {
    const latest = records.filter(r => r.valid && r.category === category).sort((a, b) => category === 'input-installer' ? b.newest - a.newest : b.generation - a.generation).slice(0, 2);
    for (const r of latest) r.reason ??= 'latest-two-valid-builds';
  }
  for (const setup of records.filter(r => r.category === 'app-setup' && r.reason)) {
    for (const bundle of records.filter(r => r.category === 'workspace' && r.generation === setup.generation && r.revision === setup.revision)) bundle.reason ??= 'retained-setup-payload';
  }
  for (const bundle of records.filter(r => r.category !== 'input-installer' && r.reason)) {
    for (const input of records.filter(r => r.category === 'input-installer' && r.revision === bundle.revision)) input.reason ??= 'retained-bundle-installer';
  }
  if (records.some(r => !r.valid && r.category !== 'input-installer')) {
    for (const r of records.filter(r => r.category === 'input-installer')) r.reason ??= 'unrecognized-consumer';
  }
  for (const r of records) if (!r.reason) { r.action = 'Remove'; r.reason = 'expired-generated-build'; }
  assertGuards(state.guards);
  return {repo, now, records, guards: state.guards, keepPaths};
}

export function applyArtifactRetention(plan, {runtimeReader = windowsRuntimeState, shouldRemove = () => true} = {}) {
  const result = {removed: [], bytes: 0};
  for (const target of plan.records.filter(r => r.action === 'Remove')) {
    if (!categories.includes(target.category) || path.dirname(target.directory) !== path.join(plan.repo, 'dist', target.category)) throw Error('Artifact escaped its owned distribution root');
    assertGuards(plan.guards);
    const state = protection(plan.repo, runtimeReader, plan.keepPaths);
    if (pinReason(target, state, installedInputRevisions(plan.records, state))) continue;
    const fresh = inspect(target.directory, target.category);
    if (!fresh.valid || JSON.stringify(fresh.files) !== JSON.stringify(target.files)) throw Error('Artifact changed after planning');
    assertGuards(state.guards);
    if (!shouldRemove(target)) continue;
    // Exact flat file set was checked above; never recursively remove directories.
    for (const file of fresh.files) fs.unlinkSync(path.join(fresh.directory, file.name));
    fs.rmdirSync(fresh.directory);
    result.removed.push(path.relative(plan.repo, fresh.directory).replaceAll(path.sep, '/'));
    result.bytes += fresh.bytes;
  }
  return result;
}

export function retainArtifacts({repoRoot = repository, keepPaths = [], force = false, runtimeReader = windowsRuntimeState} = {}) {
  const cache = path.join(repoRoot, '.cache');
  fs.mkdirSync(cache, {recursive: true}); plain(cache);
  const receipt = path.join(cache, 'artifact-retention.json'), lease = path.join(cache, 'artifact-retention.lock');
  const policy = sha256(['prune-artifacts.mjs', 'artifact-retention-state.mjs', 'read-artifact-retention-state.ps1'].map(n => fs.readFileSync(new URL(n, import.meta.url))).join(''));
  if (fs.existsSync(receipt) && !force) {
    const prior = readJSON(receipt), elapsed = Date.now() - Date.parse(prior.checkedAt);
    if (prior.schema === 'krasis.artifact-retention/v1' && prior.policy === policy && elapsed >= 0 && elapsed < 60 * 60 * 1000) return {skipped: 'hourly-throttle'};
  }
  let handle;
  try { handle = fs.openSync(lease, 'wx'); }
  catch (error) {
    if (error.code !== 'EEXIST') throw error;
    const owner = readJSON(lease);
    if (!Number.isSafeInteger(owner.pid) || owner.pid < 1) throw Error('Invalid retention lock');
    try { process.kill(owner.pid, 0); return {skipped: 'busy'}; }
    catch (status) { if (status.code !== 'ESRCH') return {skipped: 'busy'}; }
    fs.unlinkSync(lease);
    handle = fs.openSync(lease, 'wx');
  }
  try {
    fs.writeFileSync(handle, JSON.stringify({pid: process.pid}));
    const plan = planArtifactRetention({repoRoot, keepPaths, runtimeReader});
    const result = applyArtifactRetention(plan, {runtimeReader});
    const temporary = receipt + '.' + process.pid + '.tmp';
    let created = false;
    try {
      fs.writeFileSync(temporary, JSON.stringify({schema: 'krasis.artifact-retention/v1', checkedAt: new Date().toISOString(), policy, ...result}) + '\n', {flag: 'wx'});
      created = true;
      fs.renameSync(temporary, receipt);
    } finally { if (created && fs.existsSync(temporary)) fs.unlinkSync(temporary); }
    return result;
  } finally { fs.closeSync(handle); fs.unlinkSync(lease); }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  if (args.some(a => !['--apply', '--force'].includes(a) && !a.startsWith('--keep='))) throw Error('Expected --apply, --force or --keep=artifact-path');
  const keepPaths = args.filter(a => a.startsWith('--keep=')).map(a => a.slice(7));
  const result = args.includes('--apply') ? retainArtifacts({keepPaths, force: args.includes('--force')}) :
    planArtifactRetention({keepPaths}).records.map(r => ({path: path.relative(repository, r.directory).replaceAll(path.sep, '/'), action: r.action, reason: r.reason, bytes: r.bytes}));
  console.log(JSON.stringify(result, null, 2));
}
