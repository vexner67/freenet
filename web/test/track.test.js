import test from 'node:test';
import assert from 'node:assert/strict';
import { shortcut, trackAt } from '../static/track.js';

test('the part of a line under the pointer stays within the line', () => {
  assert.equal(trackAt(150, 100, 200), 0.25);
  assert.equal(trackAt(100, 100, 200), 0);
  assert.equal(trackAt(300, 100, 200), 1);
  assert.equal(trackAt(50, 100, 200), 0);
  assert.equal(trackAt(900, 100, 200), 1);
  assert.equal(trackAt(150, 100, 0), 0, 'a line not laid out yet');
});

test('the keys of the big player are openvk\'s, in any layout', () => {
  assert.deepEqual(shortcut('Space'), ['toggle']);
  assert.deepEqual(shortcut('ArrowLeft'), ['skip', -3]);
  assert.deepEqual(shortcut('ArrowRight'), ['skip', 3]);
  assert.deepEqual(shortcut('KeyA'), ['prev']);
  assert.deepEqual(shortcut('KeyW'), ['prev']);
  assert.deepEqual(shortcut('KeyD'), ['next']);
  assert.deepEqual(shortcut('KeyS'), ['next']);
  assert.deepEqual(shortcut('KeyR'), ['repeat']);
  assert.deepEqual(shortcut('KeyM'), ['mute']);
  assert.deepEqual(shortcut('NumpadAdd'), ['louder', 10]);
  assert.deepEqual(shortcut('NumpadSubtract'), ['louder', -10]);
  assert.equal(shortcut('KeyQ'), null);
});
