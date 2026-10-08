import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import assert from 'node:assert/strict';
import { verifyExecutableIcon } from './check-windows-icon.mjs';
import { signExecutable, signingEnabled } from './authenticode.mjs';

assert.equal(process.argv.length,2,'Run from the checkout without arguments.');
const git=args=>execFileSync('git',args,{encoding:'utf8'}).trim();
assert.equal(git(['status','--porcelain']),'','Commit source before packaging setup.');
const revision=git(['rev-parse','HEAD']);
const generation=Number(fs.readFileSync('internal/localapi/runtime.go','utf8').match(/const AgentGeneration = (\d+)/u)?.[1]);
const source=path.resolve('dist/workspace',`g${generation}`);
const metadata=JSON.parse(fs.readFileSync(path.join(source,'BUILD.json'),'utf8'));
assert.equal(metadata.revision,revision);assert.equal(metadata.generation,generation);
assert.equal(metadata.signed,signingEnabled(),'Setup signing mode must match its payload.');
execFileSync('node',['scripts/check-workspace-package.mjs',source],{stdio:'inherit'});
const output=path.resolve('dist/app-setup',`g${generation}`);
assert.ok(!fs.existsSync(output),'Never overwrite a setup artifact.');fs.mkdirSync(output,{recursive:true});
const payload=path.resolve('cmd/krasis-setup/payload');fs.mkdirSync(payload,{recursive:true});
const archive=path.join(payload,'bundle.zip');
execFileSync('tar',['-a','-cf',archive,'-C',source,...fs.readdirSync(source).sort()],{stdio:'inherit'});
assert.ok(fs.statSync(archive).size<=96*1024*1024);
const tools=process.env.KRASIS_LLVM_BIN||path.resolve('.cache/tools/llvm-mingw-20260908-ucrt-x86_64/bin');
const resource=path.resolve('cmd/krasis-setup/setup_windows_amd64.syso');
assert.ok(!fs.existsSync(resource));
const res=path.join(output,'setup.res');
execFileSync(path.join(tools,'llvm-rc.exe'),['/FO',res,'krasis-setup.rc'],{cwd:path.resolve('assets/windows'),stdio:'inherit'});
try {
  execFileSync(path.join(tools,'llvm-cvtres.exe'),['/MACHINE:X64','/TIMESTAMP:0',`/OUT:${resource}`,res],{stdio:'inherit'});
  const exe=path.join(output,`KrasisSetup-g${generation}.exe`);
  execFileSync('go',['build','-trimpath','-buildvcs=true','-tags','appsetup','-ldflags=-s -w -H=windowsgui','-o',exe,'./cmd/krasis-setup'],
    {env:{...process.env,GOOS:'windows',GOARCH:'amd64',CGO_ENABLED:'0',GOPROXY:'off',GOFLAGS:''},stdio:'inherit'});
  const signed=signExecutable(exe);
  verifyExecutableIcon(fs.readFileSync(exe));
  const verification=JSON.parse(execFileSync(exe,['--verify','--json'],{encoding:'utf8',timeout:30000,windowsHide:true}));
  assert.equal(verification.code,0);assert.equal(verification.state,'verified');
  const sha256=createHash('sha256').update(fs.readFileSync(exe)).digest('hex');
  fs.writeFileSync(path.join(output,'BUILD.json'),JSON.stringify({schema:'krasis.setup-build/v1',generation,revision,signed,installed:false,sha256},null,2)+'\n',{flag:'wx'});
  fs.writeFileSync(path.join(output,'SHA256SUMS.txt'),`${sha256}  ${path.basename(exe)}\n`,{flag:'wx'});
  assert.equal(git(['status','--porcelain']),'','Setup builder changed source.');
  console.log(`Application setup verified, not installed: ${exe}`);
} finally { if(fs.existsSync(resource))fs.unlinkSync(resource); }
