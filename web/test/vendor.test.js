import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const vendor = new URL('../static/vendor/livekit-client.mjs', import.meta.url);
const source = readFileSync(vendor);

test('LiveKit 2.23.0 is the unmodified official ESM distribution', () => {
  assert.equal(createHash('sha256').update(source).digest('hex'),
    '053820ce316a0d9881e54d4745eeb45b96575c05d3591a214f318168336e1367');
  assert.match(readFileSync(new URL('../static/vendor/livekit-client.LICENSE', import.meta.url), 'utf8'), /Apache License/);
});

test('LiveKit parses as an ES module and requires no imported dependencies', () => {
  execFileSync(process.execPath, ['--check', fileURLToPath(vendor)]);
  assert.doesNotMatch(source.toString(), /^import\s/m);
});

test('LiveKit contains no dynamic code or HTML sinks', () => {
  assert.doesNotMatch(source.toString(), /\beval\s*\(|new Function\s*\(|\.innerHTML\b|\.outerHTML\b|insertAdjacentHTML|document\.write|createObjectURL/);
});
