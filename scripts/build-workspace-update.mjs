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
  const ldflags = name === 'krasis-cli' ? '-s -w' : '-s -w -H=windowsgui';
  execFileSync('go',['build','-trimpath',...tags,`-ldflags=${ldflags}`,'-o',path.join(directory,`${name}.exe`),`./cmd/${name}`],{stdio:'inherit',env:{...process.env,GOOS:'windows',GOARCH:'amd64',CGO_ENABLED:'0',GOPROXY:'off',GOSUMDB:'off'}});
  verifyExecutableIcon(fs.readFileSync(path.join(directory,`${name}.exe`)));
}
fs.copyFileSync('frontend/README.md',path.join(directory,'START-HERE.md'));
fs.copyFileSync(path.join(bundleRoot,'THIRD-PARTY-NOTICES.txt'),path.join(directory,'THIRD-PARTY-NOTICES.txt'));
fs.writeFileSync(path.join(directory,'BUILD.json'),JSON.stringify({revision,generation,platform:'windows/amd64',cgo:false,purpose:'manual-two-pc-smoke',signed:false},null,2)+'\n');
const names=['krasis.exe','krasis-agent.exe','krasis-cli.exe','START-HERE.md','BUILD.json','THIRD-PARTY-NOTICES.txt'];
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
    : null;
if (uiAccessTest) {
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
    ['README.md', fs.readFileSync('experiments/uiaccess-input/README.md')],
    ['install-test.ps1', fs.readFileSync('experiments/uiaccess-input/install-test.ps1')],
    ['remove-test.ps1', fs.readFileSync('experiments/uiaccess-input/remove-test.ps1')],
  ]) {
    fs.writeFileSync(path.join(directory, name), content, { flag: 'wx' });
    names.push(name);
  }
}
fs.writeFileSync(path.join(directory,'SHA256SUMS.txt'),names.map(name=>crypto.createHash('sha256').update(fs.readFileSync(path.join(directory,name))).digest('hex')+'  '+name).join('\n')+'\n');
execFileSync('go',['test','-v','-count=1','-timeout','60s','-run','^TestWorkspaceUpdatePackage$','./internal/productpanel'],{stdio:'inherit',env:{...process.env,KRASIS_WORKSPACE_UPDATE:path.resolve(directory),KRASIS_UPDATE_REVISION:revision}});
