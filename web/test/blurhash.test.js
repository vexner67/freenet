// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { decodeBlurhash } from '../static/blurhash.js';

const digits = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~';

function encode83(value, length) {
  let text = '';
  for (let i = 0; i < length; i++) {
    text = digits[value % 83] + text;
    value = Math.floor(value / 83);
  }
  return text;
}

test('a blurhash of one color is that color everywhere', () => {
  // One component, no maximum, the color itself.
  const pixels = decodeBlurhash('00' + encode83(0xff8000, 4), 3, 2);

  assert.equal(pixels.length, 3 * 2 * 4);
  for (let at = 0; at < pixels.length; at += 4) {
    assert.deepEqual([...pixels.slice(at, at + 4)], [255, 128, 0, 255]);
  }
});

test('a blurhash of many components draws every pixel opaque', () => {
  const pixels = decodeBlurhash('LEHV6nWB2yk8pyo0adR*.7kCMdnj', 32, 18);

  assert.equal(pixels.length, 32 * 18 * 4);
  for (let at = 3; at < pixels.length; at += 4) assert.equal(pixels[at], 255);
  // Not all one color: the waves are there.
  assert.notDeepEqual([...pixels.slice(0, 3)], [...pixels.slice(-4, -1)]);
});

test('what is no blurhash draws nothing', () => {
  for (const hash of ['', 'L', 'LEHV6nWB2yk8pyo0adR*.7kCMdn', 'LEHV6nWB2yk8pyo0adR*.7kCMd\\\\', null, 42]) {
    assert.equal(decodeBlurhash(hash, 4, 4), null, String(hash));
  }
});
