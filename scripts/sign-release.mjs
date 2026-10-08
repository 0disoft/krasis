import fs from 'node:fs';
import path from 'node:path';
import { createHash, createPrivateKey, createPublicKey, sign, verify } from 'node:crypto';
import { execFileSync } from 'node:child_process';

// The private PEM is supplied by the release environment, never an artifact.
const directory=path.resolve(process.argv[2]??'');
const metadata=JSON.parse(fs.readFileSync(path.join(directory,'BUILD.json'),'utf8'));
if(metadata.signed!==true||metadata.purpose!=='windows-release')throw Error('Official releases require a publisher-signed payload');
const generation=metadata.generation;
if(!Number.isSafeInteger(generation)||generation<21||generation>1000000||directory!==path.resolve('dist/workspace',`g${generation}`)||!/^[a-f0-9]{40}$/u.test(metadata.revision))throw Error('Invalid release bundle');
if(execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim()!==metadata.revision || execFileSync('git',['status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('Release source differs from bundle');
const publicKey=process.env.KRASIS_RELEASE_PUBLIC_KEY;
if(!publicKey||!process.env.KRASIS_RELEASE_PRIVATE_KEY)throw Error('Release signing keys not configured');
const privateKey=createPrivateKey(process.env.KRASIS_RELEASE_PRIVATE_KEY);
if(privateKey.asymmetricKeyType!=='ed25519')throw Error('Expected Ed25519 release key');
const pub=createPublicKey(privateKey);
const rawPublic=pub.export({format:'der',type:'spki'}).subarray(-32).toString('base64');
if(publicKey!==rawPublic)throw Error('Release signing key differs from embedded public key');
execFileSync('node',['scripts/check-workspace-package.mjs',directory],{stdio:'inherit'});
// Execute only the locally built, package-verified UI in its read-only check mode.
// -trimpath intentionally omits -ldflags from Go's build metadata.
execFileSync(path.join(directory,'krasis.exe'),['--release-key-check',publicKey],{timeout:10000,stdio:'pipe'});
const output=path.resolve('dist/releases',`g${generation}`);
if(fs.existsSync(output))throw Error('Release artifact already exists');
fs.mkdirSync(output,{recursive:true});
const name=`krasis-g${generation}-windows-amd64.zip`;
const archive=path.join(output,name);
execFileSync('tar',['-a','-cf',archive,'-C',directory,...fs.readdirSync(directory)],{stdio:'inherit'});
const data=fs.readFileSync(archive);
if(data.length>96*1024*1024)throw Error('Release archive exceeds updater limit');
const payload=Buffer.from(JSON.stringify({schema:'krasis.release/v1',generation,revision:metadata.revision,platform:'windows/amd64',url:`https://github.com/0disoft/krasis/releases/download/g${generation}/${name}`,sha256:createHash('sha256').update(data).digest('hex'),size:data.length}));
const signature=sign(null,payload,privateKey);
if(!verify(null,payload,pub,signature))throw Error('Release signature self-check failed');
fs.writeFileSync(path.join(output,'latest.json'),JSON.stringify({payload:payload.toString('base64'),signature:signature.toString('base64')})+'\n',{flag:'wx'});
console.log(`Signed release prepared: g${generation}`);
