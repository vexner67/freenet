import { test } from 'node:test';
import assert from 'node:assert/strict';
import { cellFontSize, holdemBlinds, holdemBuyIn, holdemChips, holdemPlace } from '../static/labels.js';

test('a number keeps inside its casino cell', () => {
  assert.equal(cellFontSize('50'), 20);
  assert.equal(cellFontSize('×1,5'), 20);
  assert.equal(cellFontSize('1 000 000'), 12);
  assert.equal(cellFontSize('1 000 000 000 000'), 10);
});

test('the viewer sits at the bottom of the table, the rest clockwise', () => {
  assert.deepEqual(holdemPlace(2, 6, 2), { x: 50, y: 92 });
  assert.deepEqual(holdemPlace(0, 2), { x: 50, y: 92 });
  assert.deepEqual(holdemPlace(1, 2), { x: 50, y: 8 });
  assert.deepEqual(holdemPlace(1, 4), { x: 6, y: 50 });
});

test('a bet is drawn with its chips, the biggest first', () => {
  assert.deepEqual(holdemChips(1131), [1000, 100, 25, 5, 1]);
  assert.deepEqual(holdemChips(30), [25, 5]);
  assert.equal(holdemChips(99999).length, 6);
  assert.deepEqual(holdemChips(0), []);
});

test('the buy-in is 100 big blinds within the table and the balance', () => {
  assert.equal(holdemBuyIn(10, 5000), 1000);
  assert.equal(holdemBuyIn(10, 500), 500);
  assert.equal(holdemBuyIn(10, 50), 200);
  assert.equal(holdemBuyIn(10), 1000);
  assert.equal(holdemBlinds(1000), '500/1 000');
});
