import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { verifyExecutableIcon } from './check-windows-icon.mjs';

// The experimental probe is an optional, atomic pair. A normal product update
// must not require new test-signing evidence for a probe that is not deployed.
// No received executable or MSIX is launched by this builder.
const git = args => execFileSync('git',args,{encoding:'utf8'}).trim();
if (git(['status','--porcelain','--untracked-files=no'])) throw Error('Commit source changes before packaging updates.');
const revision = git(['rev-parse','HEAD']);
const generation = Number(fs.readFileSync('internal/localapi/runtime.go','utf8').match(/const AgentGeneration = (\d+)/u)?.[1]);
if (!Number.isSafeInteger(generation)) throw Error('Missing generation');
const bundleRoot = process.argv[2];
if (!bundleRoot || path.resolve(bundleRoot) !== path.resolve('dist/workspace', `g${generation}`)) throw Error('Expected generation-scoped distribution');
const directory = bundleRoot;
for (const name of ['krasis','krasis-agent','krasis-cli']) {
  const tags = name === 'krasis' ? ['-tags','desktop,production'] : [];
  const publicKey = process.env.KRASIS_RELEASE_PUBLIC_KEY ?? '';
  if (publicKey && (!/^[A-Za-z0-9+/]{43}=$/u.test(publicKey) || Buffer.from(publicKey,'base64').length !== 32)) throw Error('Invalid release public key');
  const ldflags = (name === 'krasis-cli' ? '-s -w' : '-s -w -H=windowsgui') + (publicKey ? ` -X=github.com/0disoft/krasis/internal/releaseupdate.PublicKey=${publicKey}` : '');
  execFileSync('go',['build','-trimpath','-buildvcs=true',...tags,`-ldflags=${ldflags}`,'-o',path.join(directory,`${name}.exe`),`./cmd/${name}`],{stdio:'inherit',env:{...process.env,GOFLAGS:'',GOOS:'windows',GOARCH:'amd64',CGO_ENABLED:'0',GOPROXY:'off',GOSUMDB:'off'}});
  verifyExecutableIcon(fs.readFileSync(path.join(directory,`${name}.exe`)));
}
fs.copyFileSync('frontend/README.md',path.join(directory,'START-HERE.md'));
fs.copyFileSync(path.join(bundleRoot,'THIRD-PARTY-NOTICES.txt'),path.join(directory,'THIRD-PARTY-NOTICES.txt'));
fs.writeFileSync(path.join(directory,'BUILD.json'),JSON.stringify({revision,generation,platform:'windows/amd64',cgo:false,purpose:'manual-two-pc-smoke',signed:false},null,2)+'\n');
const names=['krasis.exe','krasis-agent.exe','krasis-cli.exe','START-HERE.md','BUILD.json','THIRD-PARTY-NOTICES.txt'];
// g105 is an installer-free compatibility bridge. Newer bundles carry the
// installer, but neither the builder nor update receipt elevates or runs it.
if (generation >= 106) {
  const installerRoot = path.resolve('dist/input-installer', revision);
  if (!fs.existsSync(path.join(installerRoot, 'BUILD.json'))) {
    execFileSync('node', ['scripts/build-input-installer.mjs'], { stdio: 'inherit' });
  }
  const evidence = JSON.parse(fs.readFileSync(path.join(installerRoot, 'BUILD.json'), 'utf8'));
  const executable = fs.readFileSync(path.join(installerRoot, 'krasis-install.exe'));
  const digest = data => crypto.createHash('sha256').update(data).digest('hex');
  if (evidence.schemaVersion !== 'krasis.input.installer-build/v1' || evidence.revision !== revision ||
      evidence.platform !== 'windows/amd64' || evidence.executionLevel !== 'requireAdministrator' ||
      evidence.signed !== false || evidence.installed !== false || evidence.sha256 !== digest(executable) ||
      evidence.agentSha256 !== digest(fs.readFileSync(path.join(directory, 'krasis-agent.exe')))) {
    throw Error('Installer provenance or embedded agent differs from this update build');
  }
  verifyExecutableIcon(executable);
  fs.writeFileSync(path.join(directory, 'krasis-install.exe'), executable, { flag: 'wx' });
  names.push('krasis-install.exe');
}
if (generation >= 62) {
  const probeRoot = path.resolve('dist/brokered-input-probe');
  const candidates = fs.existsSync(probeRoot) ? fs.readdirSync(probeRoot,{withFileTypes:true})
    .filter(entry=>entry.isDirectory() && entry.name.startsWith(`${revision.slice(0,12)}-`))
    .map(entry=>path.join(probeRoot,entry.name)) : [];
  if (candidates.length > 1) throw Error('Ambiguous signed probes for the current source revision');
  if (candidates.length === 1) {
    const source = candidates[0];
    const packageName = 'KrasisBrokeredProbe-TESTSIGNED.msix';
    const certificateName = 'KrasisBrokeredProbe-TESTCERT.cer';
    const msix = fs.readFileSync(path.join(source,packageName));
    const certificate = fs.readFileSync(path.join(source,certificateName));
    const details = fs.readFileSync(path.join(source,'TEST-CERT-DETAILS.txt'),'utf8').replaceAll('\r','');
    const expectedHash = details.match(/^Package SHA256: ([a-fA-F0-9]{64})$/mu)?.[1]?.toLowerCase();
    const expectedThumbprint = details.match(/^Thumbprint: ([a-fA-F0-9]{40})$/mu)?.[1]?.toLowerCase();
    const parsedCertificate = new crypto.X509Certificate(certificate);
    if (crypto.createHash('sha256').update(msix).digest('hex') !== expectedHash || parsedCertificate.fingerprint.replaceAll(':','').toLowerCase() !== expectedThumbprint || Date.parse(parsedCertificate.validTo) <= Date.now()) {
      throw Error('Probe signing evidence does not match the package and certificate');
    }
    fs.writeFileSync(path.join(directory,packageName),msix,{flag:'wx'});
    fs.writeFileSync(path.join(directory,certificateName),certificate,{flag:'wx'});
    names.push(packageName,certificateName);
  }
}
// Only the listed generations carry a pinned, attended capability test.
// Other generations do not silently inherit the development payload.
// Neither building nor receiving the bundle trusts its certificate or runs it.
const uiAccessTest = generation >= 91 && generation <= 94
  ? { source: '9f73fe9980ca-20260929043811', hash: 'c4fad81215a94ff67abe99ac1985dcfbb3f5b1e6f252848156debf23c63f98c4', thumbprint: '6ab1765f3c268d3bdd64a5f2e2a7ebb2071ee036' }
  : generation === 97
    ? { source: '580af88924da-20260929100017', hash: '6cb515d5174e152910f811a12f03ce0467c1a84a1f601a516e14bcef839f005e', thumbprint: 'af3540c895b677a1a8f7fe6e8569086f6f30e493' }
    : generation === 98
      ? { source: '79c60db76d70-20260930064122', hash: '015224b317e298c572d79d07e4ed813e85d9f4a36abbf462e8029062ee4c12da', thumbprint: '539774816405d2bc1861daaeb9019829f7465b06' }
      : generation === 99
        ? { source: 'b4ebe88d74f8-20260930074811', hash: '4ba13bd4a84d84310f46564d09dea3d14c717aa40fe3ceaac99f6a7512e6575a', thumbprint: 'f9b42783461db4d3327ffb7eef580720975ab729' }
        : generation === 100
          ? { source: '562ab7072949-20260930083839', hash: '5c403f0a7ebad709c0f2c8fd780a3269b1c7a418cea8549b14f0ba1736647261', thumbprint: 'c188d613e58826bb448b05b0513a8a7fb2044ce9' }
          : generation === 101
            ? { source: '88df8821d6fe-20260930091221', hash: '19c7270577ffd77db60ceaeb7b09ff58d9ba7eb2fcf5b8dd2b7690b9b79568ff', thumbprint: 'a39bfe34d80a8cf3cba5a0482a90dc565333c4b8' }
            : generation === 102
              ? { source: 'c49f894a1eef-20260930100335', hash: '35b0e2be4dc85be7044ac1cabf03a6f44ab78a9a22db03b5d4734962ce6471ce', thumbprint: '2fc5626f2874eb71ae5db7bd6eaedbf3ad853b58' }
              : generation === 103
                ? { source: '9b0bff07bb19-20260930125759', hash: 'b7e6e3ecb6e4d63060849a3b3fe4c42ae4dd4d0817aa25c3b783cbf375a42863', thumbprint: 'f0417926f4b338da41484779960350bfc064c543' }
                : generation === 104
                  ? { source: '5d4eaee0dc55-20260930133520', hash: 'a42cdc98adee2ed122b0fc3752750264cbf9a0d0fc6a6f20aac122a2fe49a349', thumbprint: '272c0e5a14037879cb5c50148c82639d27380c40' }
                  : null;
if (uiAccessTest) {
  const templateRoot = [99,100,101,102,103,104].includes(generation) ? 'experiments/system-input' : 'experiments/uiaccess-input';
  const source = path.resolve('dist/uiaccess-input-probe', uiAccessTest.source);
  const executableName = 'KrasisUIAccessProbe-TESTSIGNED.exe';
  const certificateName = 'KrasisUIAccessProbe-TESTCERT.cer';
  const detailsName = 'TEST-CERT-DETAILS.txt';
  const expectedHash = uiAccessTest.hash;
  const expectedThumbprint = uiAccessTest.thumbprint;
  const executable = fs.readFileSync(path.join(source, executableName));
  const certificate = fs.readFileSync(path.join(source, certificateName));
  const details = fs.readFileSync(path.join(source, detailsName), 'utf8').replaceAll('\r', '');
  const parsedCertificate = new crypto.X509Certificate(certificate);
  if (crypto.createHash('sha256').update(executable).digest('hex') !== expectedHash ||
      parsedCertificate.fingerprint.replaceAll(':', '').toLowerCase() !== expectedThumbprint ||
      Date.parse(parsedCertificate.validTo) <= Date.now() ||
      details.match(/^EXE SHA256: ([a-f0-9]{64})$/mu)?.[1] !== expectedHash ||
      details.match(/^Thumbprint: ([A-F0-9]{40})$/mu)?.[1]?.toLowerCase() !== expectedThumbprint) {
    throw Error('UIAccess test artifact does not match pinned signing evidence');
  }
  for (const [name, content] of [
    [executableName, executable], [certificateName, certificate], [detailsName, Buffer.from(details)],
    ['README.md', fs.readFileSync(path.join(templateRoot, 'README.md'))],
    ['install-test.ps1', fs.readFileSync(path.join(templateRoot, 'install-test.ps1'))],
    ['remove-test.ps1', fs.readFileSync(path.join(templateRoot, 'remove-test.ps1'))],
  ]) {
    fs.writeFileSync(path.join(directory, name), content, { flag: 'wx' });
    names.push(name);
  }
}
fs.writeFileSync(path.join(directory,'SHA256SUMS.txt'),names.map(name=>crypto.createHash('sha256').update(fs.readFileSync(path.join(directory,name))).digest('hex')+'  '+name).join('\n')+'\n');
execFileSync('go',['test','-v','-count=1','-timeout','60s','-run','^TestWorkspaceUpdatePackage$','./internal/productpanel'],{stdio:'inherit',env:{...process.env,KRASIS_WORKSPACE_UPDATE:path.resolve(directory),KRASIS_UPDATE_REVISION:revision}});
