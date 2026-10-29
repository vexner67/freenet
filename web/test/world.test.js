// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { worldPopulation } from '../static/labels.js';
import { cellAt, clampView, tileZoom, visibleTiles } from '../static/world-map.js';

test('tileZoom takes the zoom whose tiles are as sharp as the screen', () => {
  assert.equal(tileZoom(0.25, 2048), 0);
  assert.equal(tileZoom(0.5, 2048), 1);
  assert.equal(tileZoom(1, 2048), 2);
  assert.equal(tileZoom(8, 2048, 3), 3);
});

test('visibleTiles are the tiles in view and no further', () => {
  // At zoom 1 of a 2048×1024 map a tile is 512 cells.
  assert.deepEqual(visibleTiles({ x: 0, y: 0, width: 600, height: 200, scale: 1 }, 1, 2048, 1024), [[0, 0], [1, 0]]);
  assert.deepEqual(visibleTiles({ x: 1900, y: 900, width: 1000, height: 1000, scale: 1 }, 1, 2048, 1024), [[3, 1]]);
});

test('cellAt is the cell under a point or none outside the map', () => {
  const view = { x: 10, y: 20, scale: 2 };
  assert.deepEqual(cellAt(view, 5, 9, 100, 100), { x: 12, y: 24 });
  assert.equal(cellAt(view, 500, 0, 100, 100), null);
});

test('clampView keeps the map in the frame', () => {
  const v = clampView({ x: -50, y: 900, width: 400, height: 200, scale: 0.01 }, 2048, 1024);
  assert.equal(v.scale, Math.min(400 / 2048, 200 / 1024));
  assert.ok(v.x >= 0 && v.y >= 0);
});

test('worldPopulation says thousands and millions', () => {
  assert.equal(worldPopulation(20), '20 тыс. чел.');
  assert.equal(worldPopulation(1250), '1,3 млн чел.');
});
