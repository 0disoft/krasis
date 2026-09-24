import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { verifyExecutableIcon } from './check-windows-icon.mjs';

// g61 keeps the legacy seven-file format. g62+ adds a fixed, inert probe pair.
// No received executable or MSIX is launched by this builder.
const git = args => execFileSync('git',args,{encoding:'utf8'}).trim();
if (git(['status','--porcelain','--untracked-files=no'])) throw Error('Commit source changes before packaging updates.');
const revision = git(['rev-parse','HEAD']);
const generation = Number(fs.readFileSync('internal/localapi/runtime.go','utf8').match(/const AgentGeneration = (\d+)/u)?.[1]);
if (!Number.isSafeInteger(generation)) throw Error('Missing generation');
const bundleRoot = process.argv[2];
if (!bundleRoot || path.resolve(bundleRoot) !== path.resolve('dist/workspace', `g${generation}`)) throw Error('Expected generation-scoped distribution');
const directory = path.join(bundleRoot,'peer-update');
fs.mkdirSync(directory,{recursive:true});
for (const name of ['krasis','krasis-agent','krasis-control']) {
  const tags = name === 'krasis-control' ? ['-tags','desktop,production'] : [];
  execFileSync('go',['build','-trimpath',...tags,'-ldflags=-s -w -H=windowsgui','-o',path.join(directory,`${name}.exe`),`./cmd/${name}`],{stdio:'inherit',env:{...process.env,GOOS:'windows',GOARCH:'amd64',CGO_ENABLED:'0',GOPROXY:'off',GOSUMDB:'off'}});
  verifyExecutableIcon(fs.readFileSync(path.join(directory,`${name}.exe`)));
}
fs.copyFileSync('frontend/README.md',path.join(directory,'START-HERE.md'));
fs.copyFileSync(path.join(bundleRoot,'THIRD-PARTY-NOTICES.txt'),path.join(directory,'THIRD-PARTY-NOTICES.txt'));
fs.writeFileSync(path.join(directory,'BUILD.json'),JSON.stringify({revision,generation,platform:'windows/amd64',cgo:false,purpose:'manual-two-pc-smoke',signed:false},null,2)+'\n');
const names=['krasis.exe','krasis-agent.exe','krasis-control.exe','START-HERE.md','BUILD.json','THIRD-PARTY-NOTICES.txt'];
if (generation >= 62) {
  const probeRoot = path.resolve('dist/brokered-input-probe');
  const candidates = fs.readdirSync(probeRoot,{withFileTypes:true})
    .filter(entry=>entry.isDirectory() && entry.name.startsWith(`${revision.slice(0,12)}-`))
    .map(entry=>path.join(probeRoot,entry.name))
    .filter(candidate=>fs.existsSync(path.join(candidate,'KrasisBrokeredProbe-TESTSIGNED.msix')) && fs.existsSync(path.join(candidate,'KrasisBrokeredProbe-TESTCERT.cer')));
  if (candidates.length !== 1) throw Error('Expected exactly one signed probe for the current source revision');
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
fs.writeFileSync(path.join(directory,'SHA256SUMS.txt'),names.map(name=>crypto.createHash('sha256').update(fs.readFileSync(path.join(directory,name))).digest('hex')+'  '+name).join('\n')+'\n');
execFileSync('go',['test','-v','-count=1','-timeout','60s','-run','^TestWorkspaceUpdatePackage$','./internal/productpanel'],{stdio:'inherit',env:{...process.env,KRASIS_WORKSPACE_UPDATE:path.resolve(directory),KRASIS_UPDATE_REVISION:revision}});
