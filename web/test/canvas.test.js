import test from 'node:test';
import assert from 'node:assert/strict';

import { canvasCellAt, canvasColors, canvasPicture, datingGoalName, datingGoals, datingSeekName, datingSeeks } from '../static/labels.js';

test('the canvas has the 16 colors of the user service', () => {
  assert.equal(canvasColors.length, 16);
  assert.ok(canvasColors.every((c) => /^#[0-9a-f]{6}$/.test(c)));
});

test('the cells of the canvas become the pixels of a picture', () => {
  const rgba = canvasPicture(btoa(String.fromCharCode(0, 5, 3, 99)));

  assert.deepEqual([...rgba.slice(0, 4)], [255, 255, 255, 255]);
  assert.deepEqual([...rgba.slice(4, 8)], [0xe5, 0, 0, 255]);
  assert.deepEqual([...rgba.slice(8, 12)], [0x22, 0x22, 0x22, 255]);
  // A color there is not is drawn white.
  assert.deepEqual([...rgba.slice(12, 16)], [255, 255, 255, 255]);
});

test('a point of the canvas shown at any size is its cell', () => {
  assert.deepEqual(canvasCellAt(0, 0, 500, 100), { x: 0, y: 0 });
  assert.deepEqual(canvasCellAt(4.9, 5, 500, 100), { x: 0, y: 1 });
  assert.deepEqual(canvasCellAt(343, 171.5, 343, 100), { x: 99, y: 50 });
  assert.deepEqual(canvasCellAt(-3, 2000, 1000, 100), { x: 0, y: 99 });
});

test('every goal and gender of dating has a name', () => {
  for (const goal of datingGoals) assert.notEqual(datingGoalName(goal), goal);
  for (const seek of datingSeeks) assert.notEqual(datingSeekName(seek), seek);
});
