import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { verifyExecutableIcon } from './check-windows-icon.mjs';

// Preserve the seven-file format accepted by g34, but compile its control
// entry point with the shared Workspace UI. No executable is launched.
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
fs.writeFileSync(path.join(directory,'SHA256SUMS.txt'),names.map(name=>crypto.createHash('sha256').update(fs.readFileSync(path.join(directory,name))).digest('hex')+'  '+name).join('\n')+'\n');
execFileSync('go',['test','-v','-count=1','-timeout','60s','-run','^TestWorkspaceUpdatePackage$','./internal/productpanel'],{stdio:'inherit',env:{...process.env,KRASIS_WORKSPACE_UPDATE:path.resolve(directory),KRASIS_UPDATE_REVISION:revision}});
