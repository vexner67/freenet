import assert from 'node:assert/strict';
import { test } from 'node:test';

import { cropSquare, fitInside, movedSquare, resizedSquare } from '../static/crop.js';

test('fitInside keeps proportions and does not grow unless asked', () => {
  assert.deepEqual(fitInside(4000, 3000, 1600, 1600), { width: 1600, height: 1200 });
  assert.deepEqual(fitInside(300, 600, 440, 300), { width: 150, height: 300 });
  assert.deepEqual(fitInside(100, 50, 440, 300), { width: 100, height: 50 });
  assert.deepEqual(fitInside(100, 50, 440, 300, { grow: true }), { width: 440, height: 220 });
});

test('cropSquare offers the middle four fifths of the shorter side', () => {
  assert.deepEqual(cropSquare(1000, 500), { x: 300, y: 50, side: 400 });
  assert.deepEqual(cropSquare(500, 500), { x: 50, y: 50, side: 400 });
});

test('movedSquare stays inside the picture', () => {
  const bounds = { width: 1000, height: 500 };
  const square = { x: 300, y: 50, side: 400 };

  assert.deepEqual(movedSquare(square, 100, 20, bounds), { x: 400, y: 70, side: 400 });
  assert.deepEqual(movedSquare(square, -900, -900, bounds), { x: 0, y: 0, side: 400 });
  assert.deepEqual(movedSquare(square, 900, 900, bounds), { x: 600, y: 100, side: 400 });
});

test('resizedSquare grows from its corner, within the picture and the minimum', () => {
  const bounds = { width: 1000, height: 500 };
  const square = { x: 300, y: 50, side: 400 };

  assert.deepEqual(resizedSquare(square, 30, bounds, 50), { x: 300, y: 50, side: 430 });
  assert.equal(resizedSquare(square, 300, bounds, 50).side, 450, 'no further than the bottom');
  assert.equal(resizedSquare(square, -1000, bounds, 50).side, 50, 'no smaller than the minimum');
  assert.equal(resizedSquare({ x: 0, y: 0, side: 30 }, -10, { width: 30, height: 30 }, 50).side, 30, 'a tiny picture is all there is');
});
