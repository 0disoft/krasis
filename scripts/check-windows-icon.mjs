import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sizes = [16, 20, 24, 32, 40, 48, 64, 128, 256];
const icon = () => fs.readFileSync(path.join(root, 'assets/windows/krasis.ico'));

function frames(bytes) {
  assert.equal(bytes.readUInt32LE(0), 0x10000);
  assert.equal(bytes.readUInt16LE(4), sizes.length);
  return sizes.map((size, i) => {
    const entry = bytes.subarray(6 + i * 16, 22 + i * 16);
    assert.equal(entry[0] || 256, size);
    assert.equal(entry[1] || 256, size);
    const offset = entry.readUInt32LE(12), length = entry.readUInt32LE(8);
    assert.ok(offset >= 6 + 16 * sizes.length && offset + length <= bytes.length);
    const png = bytes.subarray(offset, offset + length);
    assert.equal(png.subarray(0, 8).toString('hex'), '89504e470d0a1a0a');
    assert.equal(png.readUInt32BE(16), size);
    assert.equal(png.readUInt32BE(20), size);
    return { entry, png };
  });
}

export function verifyExecutableIcon(bytes) {
  const pe = bytes.readUInt32LE(0x3c), optional = pe + 24;
  assert.equal(bytes.readUInt32LE(pe), 0x4550);
  assert.equal(bytes.readUInt16LE(optional), 0x20b);
  const sectionCount = bytes.readUInt16LE(pe + 6);
  const sections = optional + bytes.readUInt16LE(pe + 20);
  const rvaOffset = rva => {
    for (let i = 0; i < sectionCount; i++) {
      const at = sections + i * 40, start = bytes.readUInt32LE(at + 12);
      const length = bytes.readUInt32LE(at + 16);
      if (rva >= start && rva - start < length) return bytes.readUInt32LE(at + 20) + rva - start;
    }
    throw new Error('Icon resource RVA is outside file sections.');
  };
  const resourceRVA = bytes.readUInt32LE(optional + 112 + 16);
  assert.notEqual(resourceRVA, 0, 'Executable has no resources.');
  const base = rvaOffset(resourceRVA);
  const child = (directory, id) => {
    const count = bytes.readUInt16LE(directory + 12) + bytes.readUInt16LE(directory + 14);
    for (let i = 0; i < count; i++) {
      const at = directory + 16 + 8 * i, name = bytes.readUInt32LE(at);
      if (id === undefined || name === id) return bytes.readUInt32LE(at + 4);
    }
    throw new Error(`Missing icon resource ${id}.`);
  };
  const dir = value => {
    assert.ok(value & 0x80000000, 'Expected icon resource directory.');
    return base + (value & 0x7fffffff);
  };
  const resource = (type, id) => {
    const language = child(dir(child(dir(child(base, type)), id)));
    assert.equal(language & 0x80000000, 0);
    const data = base + language;
    const offset = rvaOffset(bytes.readUInt32LE(data)), length = bytes.readUInt32LE(data + 4);
    assert.ok(offset + length <= bytes.length);
    return bytes.subarray(offset, offset + length);
  };
  const group = resource(14, 1), expected = frames(icon());
  assert.equal(group.readUInt16LE(4), expected.length);
  expected.forEach(({ entry, png }, i) => {
    const at = 6 + i * 14;
    assert.deepEqual(group.subarray(at, at + 12), entry.subarray(0, 12));
    assert.deepEqual(resource(3, group.readUInt16LE(at + 12)), png);
  });
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const source = fs.readFileSync(path.join(root, 'assets/windows/krasis-source.png'));
  assert.equal(createHash('sha256').update(source).digest('hex'), '2d895aec4e61cfaa42d055f72e4666a88dbb5c0303d402798c562329956c4a8f');
  frames(icon());
  const objects = ['krasis', 'krasis-agent', 'krasis-control', 'krasis-workspace'].map(name => fs.readFileSync(path.join(root, `cmd/${name}/icon_windows_amd64.syso`)));
  assert.equal(objects[0].readUInt16LE(0), 0x8664);
  assert.equal(objects[0].readUInt32LE(4), 0);
  objects.slice(1).forEach(object => assert.deepEqual(object, objects[0]));
  console.log('Selected source, 9 ICO frames and matching deterministic x64 resources verified.');
}
