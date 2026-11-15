import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {planArtifactRetention, applyArtifactRetention, retainArtifacts} from './prune-artifacts.mjs';
import {digestFile, sha256} from './artifact-retention-state.mjs';

const emptyState = () => ({generations: [], revisions: [], paths: [], inputHashes: [], guards: new Map()});
function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'krasis-retention-'));
  t.after(() => {
    assert.equal(path.dirname(path.resolve(root)), path.resolve(os.tmpdir()));
    assert.match(path.basename(root), /^krasis-retention-/u);
    fs.rmSync(root, {recursive: true, force: true});
  });
  const write = (file, bytes) => { fs.mkdirSync(path.dirname(file), {recursive: true}); fs.writeFileSync(file, bytes); };
  write(path.join(root, 'internal/localapi/runtime.go'), 'package localapi\nconst AgentGeneration = 600\n');
  const revisions = new Map();
  const put = (category, generation) => {
    const revision = generation.toString(16).padStart(40, '0');
    revisions.set(generation, revision);
    const directory = path.join(root, 'dist', category, category === 'input-installer' ? revision : 'g' + generation);
    fs.mkdirSync(directory, {recursive: true});
    let metadata = {generation, revision, platform: 'windows/amd64'};
    if (category === 'workspace') {
      for (const name of ['START-HERE.md', 'THIRD-PARTY-NOTICES.txt', 'krasis.exe', 'krasis-agent.exe', 'krasis-cli.exe', 'krasis-install.exe']) write(path.join(directory, name), name + generation);
    } else if (category === 'app-setup') {
      write(path.join(directory, 'KrasisSetup-g' + generation + '.exe'), 'setup' + generation);
      write(path.join(directory, 'setup.res'), 'generated resource');
      metadata = {...metadata, schema: 'krasis.setup-build/v1', sha256: digestFile(path.join(directory, 'KrasisSetup-g' + generation + '.exe'))};
    } else {
      write(path.join(directory, 'krasis-install.exe'), 'installer' + generation);
      write(path.join(directory, 'installer.res'), 'generated resource');
      metadata = {...metadata, schemaVersion: 'krasis.input.installer-build/v1', executionLevel: 'requireAdministrator', installed: false,
        sha256: digestFile(path.join(directory, 'krasis-install.exe')), agentSha256: sha256('agent' + generation), inputSha256: sha256('input' + generation)};
    }
    write(path.join(directory, 'BUILD.json'), JSON.stringify(metadata));
    if (category !== 'input-installer') {
      const names = category === 'workspace' ? fs.readdirSync(directory).sort() : ['KrasisSetup-g' + generation + '.exe'];
      write(path.join(directory, 'SHA256SUMS.txt'), names.map(n => digestFile(path.join(directory, n)) + '  ' + n).join('\n') + '\n');
    }
    return directory;
  };
  for (const g of [520, 527, 528, 529, 530, 535, 536, 543, 544]) for (const category of ['workspace', 'app-setup', 'input-installer']) put(category, g);
  const runtimeReader = () => ({...emptyState(), generations: [544, 543]});
  const options = {repoRoot: root, now: Date.now() + 7 * 86400000, runtimeReader};
  return {root, write, put, revisions, runtimeReader, options};
}
const item = (f, category, generation) => path.join(f.root, 'dist', category, category === 'input-installer' ? f.revisions.get(generation) : 'g' + generation);

test('48-hour expiration, exact dry run/removal, g528, live/rollback and transitive installer pins', t => {
  const f = fixture(t);
  const proof = path.join(f.root, 'private-data/ledger.db'); f.write(proof, 'preserved ledger proof');
  const plan = planArtifactRetention(f.options);
  assert.ok(plan.records.filter(r => r.action === 'Remove').length >= 8);
  for (const category of ['workspace', 'app-setup', 'input-installer']) for (const g of [528, 535, 536, 543, 544]) assert.equal(plan.records.find(r => r.directory === item(f, category, g)).action, 'Keep');
  assert.deepEqual(applyArtifactRetention(plan, {runtimeReader: f.runtimeReader, shouldRemove: () => false}), {removed: [], bytes: 0});
  assert.ok(fs.existsSync(item(f, 'workspace', 520)));
  const result = applyArtifactRetention(plan, {runtimeReader: f.runtimeReader});
  assert.ok(result.bytes > 0); assert.ok(result.removed.length >= 8);
  for (const category of ['workspace', 'app-setup', 'input-installer']) for (const g of [528, 535, 536, 543, 544]) assert.ok(fs.existsSync(item(f, category, g)));
  assert.equal(fs.readFileSync(proof, 'utf8'), 'preserved ledger proof');
});

test('new copies get creation-time grace; a recently modified payload also protects its whole bundle', t => {
  const f = fixture(t);
  for (const entry of fs.readdirSync(item(f, 'workspace', 520))) {
    const past = new Date(Date.now() - 7 * 86400000); fs.utimesSync(path.join(item(f, 'workspace', 520), entry), past, past);
  }
  assert.equal(planArtifactRetention({...f.options, now: Date.now()}).records.find(r => r.directory === item(f, 'workspace', 520)).action, 'Keep');
  const recent = new Date(f.options.now - 86400000);
  fs.utimesSync(path.join(item(f, 'workspace', 520), 'krasis.exe'), recent, recent);
  assert.equal(planArtifactRetention(f.options).records.find(r => r.directory === item(f, 'workspace', 520)).action, 'Keep');
});

test('unknown data stays; a corrupt newest bundle cannot displace two valid recovery builds', t => {
  const f = fixture(t);
  f.write(path.join(item(f, 'workspace', 520), 'book.db'), 'user data');
  const corrupt = f.put('workspace', 605); f.write(path.join(corrupt, 'krasis.exe'), 'changed');
  const plan = planArtifactRetention({...f.options, runtimeReader: emptyState});
  assert.equal(plan.records.find(r => r.directory === corrupt).valid, false);
  assert.equal(plan.records.find(r => r.directory === item(f, 'workspace', 520)).action, 'Keep');
  for (const g of [543, 544]) assert.equal(plan.records.find(r => r.directory === item(f, 'workspace', g)).action, 'Keep');
});

test('installed protected-input hashes preserve their installer and matching workspace', t => {
  const f = fixture(t);
  const runtimeReader = () => ({...emptyState(), inputHashes: [sha256('agent530'), sha256('input530')]});
  const plan = planArtifactRetention({...f.options, runtimeReader});
  for (const category of ['workspace', 'app-setup', 'input-installer']) assert.equal(plan.records.find(r => r.directory === item(f, category, 530)).action, 'Keep');
  assert.throws(() => planArtifactRetention({...f.options, runtimeReader: () => ({...emptyState(), inputHashes: [sha256('unknown input')]})}), /Cannot identify/u);
});

test('new live consumers are rechecked; changed references or artifacts stop deletion', t => {
  const f = fixture(t), plan = planArtifactRetention(f.options);
  const runtimeReader = () => ({...f.runtimeReader(), paths: [path.join(item(f, 'workspace', 520), 'krasis.exe')]});
  applyArtifactRetention(plan, {runtimeReader});
  assert.ok(fs.existsSync(item(f, 'workspace', 520)));
  const changed = planArtifactRetention(f.options);
  f.write(path.join(item(f, 'workspace', 520), 'krasis.exe'), 'changed after plan');
  assert.throws(() => applyArtifactRetention(changed, {runtimeReader: f.runtimeReader}), /Artifact changed/u);
});

test('manual pins, malformed JSON, late pin creation and linked roots fail safely', t => {
  const f = fixture(t), pins = path.join(f.root, 'dist/artifact-retention-pins.json');
  const original = planArtifactRetention(f.options);
  f.write(pins, JSON.stringify({schema: 'krasis.artifact-retention-pins/v1', generations: [520]}));
  assert.throws(() => applyArtifactRetention(original, {runtimeReader: f.runtimeReader}), /New retention references/u);
  assert.equal(planArtifactRetention(f.options).records.find(r => r.directory === item(f, 'workspace', 520)).action, 'Keep');
  f.write(pins, '{bad'); assert.throws(() => planArtifactRetention(f.options)); fs.unlinkSync(pins);
  const target = path.join(f.root, 'outside'); fs.mkdirSync(target);
  const linked = path.join(f.root, 'dist/workspace/g777'); fs.symlinkSync(target, linked, process.platform === 'win32' ? 'junction' : 'dir');
  assert.throws(() => planArtifactRetention(f.options), /Linked/u);
  fs.unlinkSync(linked);
});

test('automatic receipt throttles work, forced invocation runs, and a live lease prevents overlap', t => {
  const f = fixture(t), options = {repoRoot: f.root, runtimeReader: f.runtimeReader};
  assert.deepEqual(retainArtifacts(options), {removed: [], bytes: 0});
  assert.equal(retainArtifacts(options).skipped, 'hourly-throttle');
  assert.deepEqual(retainArtifacts({...options, force: true}), {removed: [], bytes: 0});
  const lock = path.join(f.root, '.cache/artifact-retention.lock'); f.write(lock, JSON.stringify({pid: process.pid}));
  assert.equal(retainArtifacts({...options, force: true}).skipped, 'busy');
});

test('retained review builds keep their input dependency and arbitrary deletion roots are rejected', t => {
  const f = fixture(t), metadata = fs.readFileSync(path.join(item(f, 'workspace', 520), 'BUILD.json'));
  f.write(path.join(f.root, 'dist/review', f.revisions.get(520), 'g520/BUILD.json'), metadata);
  const plan = planArtifactRetention(f.options);
  for (const category of ['workspace', 'input-installer']) assert.equal(plan.records.find(r => r.directory === item(f, category, 520)).action, 'Keep');
  const target = plan.records.find(r => r.action === 'Remove'); target.directory = f.root;
  assert.throws(() => applyArtifactRetention(plan, {runtimeReader: f.runtimeReader}), /escaped/u);
  fs.unlinkSync(path.join(f.root, 'dist/review', f.revisions.get(520), 'g520/BUILD.json'));
  assert.equal(planArtifactRetention(f.options).records.find(r => r.directory === item(f, 'input-installer', 520)).action, 'Keep');
});
