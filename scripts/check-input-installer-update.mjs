import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';

// Exercise actual Pack -> Read -> Stage -> UI launch preparation with an
// embedded installer. This cache fixture is never offered to a peer or run.
assert.equal(process.argv.length, 2);
assert.equal(execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim(), '');
const revision = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
assert.match(revision, /^[a-f0-9]{40}$/u);
const generation = Number(fs.readFileSync('internal/localapi/runtime.go', 'utf8').match(/const AgentGeneration = (\d+)/u)?.[1]);
assert.ok(generation >= 105);
const source = path.resolve('dist/workspace', `g${generation}`);
const installerRoot = path.resolve('dist/input-installer', revision);
const evidence = JSON.parse(fs.readFileSync(path.join(installerRoot, 'BUILD.json'), 'utf8'));
const installer = fs.readFileSync(path.join(installerRoot, 'krasis-install.exe'));
const digest = data => createHash('sha256').update(data).digest('hex');
assert.equal(evidence.revision, revision);
assert.equal(evidence.sha256, digest(installer));
assert.equal(evidence.agentSha256, digest(fs.readFileSync(path.join(source, 'krasis-agent.exe'))));
const output = path.resolve('.cache/input-installer-update', revision);
assert.ok(!fs.existsSync(output), 'Never overwrite an existing fixture.');
fs.mkdirSync(output, { recursive: true });
const names = ['krasis.exe', 'krasis-agent.exe', 'krasis-cli.exe', 'START-HERE.md', 'BUILD.json', 'THIRD-PARTY-NOTICES.txt'];
for (const name of names) fs.writeFileSync(path.join(output, name), fs.readFileSync(path.join(source, name)), { flag: 'wx' });
fs.writeFileSync(path.join(output, 'krasis-install.exe'), installer, { flag: 'wx' });
names.push('krasis-install.exe');
fs.writeFileSync(path.join(output, 'SHA256SUMS.txt'), names.map(name => `${digest(fs.readFileSync(path.join(output, name)))}  ${name}`).join('\n') + '\n', { flag: 'wx' });
execFileSync('go', ['test', '-count=1', '-timeout', '60s', '-run', '^TestWorkspaceUpdatePackage$', './internal/productpanel'], {
  stdio: 'inherit', env: { ...process.env, KRASIS_WORKSPACE_UPDATE: output, KRASIS_UPDATE_REVISION: revision },
});
// Run the real g104 parser from the committed predecessor in an isolated,
// dependency-free module. It must accept g105 and reject the added installer.
const legacy = path.join(output, 'g104-validator');
fs.mkdirSync(legacy);
fs.writeFileSync(path.join(legacy, 'go.mod'), 'module krasis-g104-update-validator\n\ngo 1.26.0\n', { flag: 'wx' });
fs.writeFileSync(path.join(legacy, 'bundle.go'), execFileSync('git', [
  'show', '530a5d255c256194370adeed409aea3b1fc04b76:internal/updatebundle/bundle.go',
]), { flag: 'wx' });
fs.writeFileSync(path.join(legacy, 'compatibility_test.go'), `package updatebundle
import("errors";"os";"testing")
func TestG104ReceiverCompatibility(t *testing.T) {
  for _, test := range []struct{directory string;accept bool}{
    {os.Getenv("KRASIS_BRIDGE_FIXTURE"),true},{os.Getenv("KRASIS_INSTALLER_FIXTURE"),false},
  } {
    root,err:=os.OpenRoot(test.directory);if err!=nil{t.Fatal(err)}
    _,err=Pack(root);root.Close()
    if test.accept && err!=nil {t.Fatal("g104 rejected g105 bridge",err)}
    if !test.accept && !errors.Is(err,ErrInvalid){t.Fatal("g104 did not reject installer layout",err)}
  }
}
`, { flag: 'wx' });
execFileSync('go', ['test', '-count=1', '-timeout', '60s', './...'], {
  cwd: legacy, stdio: 'inherit', env: {
    ...process.env, GOWORK: 'off', KRASIS_BRIDGE_FIXTURE: source, KRASIS_INSTALLER_FIXTURE: output,
  },
});
process.stdout.write('Installer update fixture verified without running installer or UI.\n');
