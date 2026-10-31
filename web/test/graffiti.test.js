import { test } from 'node:test';
import assert from 'node:assert/strict';

import { canvasPoint, drawStrokes, floodFill, stroke } from '../static/graffiti.js';

// recorder is a 2D context that writes down what is drawn.
function recorder() {
  const calls = [];
  const ctx = new Proxy({}, {
    get: (_, name) => (...args) => calls.push([name, ...args]),
    set: (_, name, value) => {
      calls.push(['=' + String(name), value]);
      return true;
    },
  });

  return { ctx, calls };
}

test('a click is a dot as wide as the pen', () => {
  const { ctx, calls } = recorder();

  drawStrokes(ctx, [stroke('#f00', 10, 0.5, { x: 20, y: 30 })], 2);

  assert.deepEqual(calls.find((c) => c[0] === 'arc'), ['arc', 40, 60, 10, 0, Math.PI * 2]);
  assert.ok(calls.some((c) => c[0] === 'fill'));
  assert.deepEqual(calls.find((c) => c[0] === '=globalAlpha'), ['=globalAlpha', 0.5]);
});

test('a line goes through the middles of its points, scaled', () => {
  const { ctx, calls } = recorder();
  const s = stroke('#000', 4, 1, { x: 0, y: 0 });
  s.points.push({ x: 10, y: 0 }, { x: 20, y: 10 }, { x: 30, y: 10 });

  drawStrokes(ctx, [s], 2);

  assert.deepEqual(calls.filter((c) => ['moveTo', 'quadraticCurveTo', 'lineTo'].includes(c[0])), [
    ['moveTo', 0, 0],
    ['quadraticCurveTo', 20, 0, 30, 10],
    ['quadraticCurveTo', 40, 20, 50, 20],
    ['lineTo', 60, 20],
  ]);
  assert.deepEqual(calls.find((c) => c[0] === '=lineWidth'), ['=lineWidth', 8]);
});

test('a point of the page is a pixel of the canvas', () => {
  const canvas = { width: 560, height: 280, getBoundingClientRect: () => ({ left: 100, top: 50, width: 280, height: 140 }) };

  assert.deepEqual(canvasPoint({ clientX: 240, clientY: 120 }, canvas), { x: 280, y: 140 });
});

test('a fill stays inside the line around it', () => {
  // 5×3 white pixels with a black column in the middle.
  const pixels = new Uint8ClampedArray(5 * 3 * 4).fill(255);
  for (const y of [0, 1, 2]) pixels.fill(0, (y * 5 + 2) * 4, (y * 5 + 2) * 4 + 3);

  floodFill(pixels, 5, 3, 0, 1, [255, 0, 0]);

  const at = (x, y) => [...pixels.slice((y * 5 + x) * 4, (y * 5 + x) * 4 + 3)];
  assert.deepEqual(at(0, 0), [255, 0, 0]);
  assert.deepEqual(at(1, 2), [255, 0, 0]);
  assert.deepEqual(at(2, 1), [0, 0, 0]);
  assert.deepEqual(at(3, 1), [255, 255, 255]);
});

test('a fill mixes in by its opacity', () => {
  const pixels = new Uint8ClampedArray(4).fill(255);

  floodFill(pixels, 1, 1, 0, 0, [55, 55, 55], 0.5);

  assert.equal(pixels[0], 155);
});
