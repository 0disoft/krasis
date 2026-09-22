import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { verifyExecutableIcon } from './check-windows-icon.mjs';

const directory=process.argv[2];
assert.ok(directory,'Expected distribution directory');
const launcher=fs.readFileSync(path.join(directory,'krasis-workspace.exe'));
verifyExecutableIcon(launcher);
assert.ok(launcher.length < 5*1024*1024,'Launcher unexpectedly includes the full UI');
assert.ok(!fs.existsSync(path.join(directory,'krasis-agent.exe')),'Duplicate agent in distribution');
assert.deepEqual(fs.readdirSync(path.join(directory,'peer-update')).sort(),['BUILD.json','SHA256SUMS.txt','START-HERE.md','THIRD-PARTY-NOTICES.txt','krasis-agent.exe','krasis-control.exe','krasis.exe'].sort());
console.log('Launcher icon and single-copy seven-file update bundle verified.');
