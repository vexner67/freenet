// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { COLORS, fitFlag, flagEmblems, flagPatterns, flagShapes, randomFlag } from '../static/flag.js';
import { flagEmblemNames, flagPatternNames, governments, worldEventParts, worldPopulation, worldRanking, worldRoles } from '../static/labels.js';
import { cellAt, clampView, stateCenters, tileZoom, visibleTiles } from '../static/world-map.js';
import { colonizable, distances, foundable, mayAnswer, mayColonize, ownerOf } from '../static/world-rules.js';

const domain = readFileSync(new URL('../../world/internal/world/flag.go', import.meta.url), 'utf8');

test('the flags are the patterns and emblems the world service takes', () => {
  const counts = /var patternColors = map\[Pattern\]int\{([\s\S]*?)\n\}/.exec(domain)[1];
  const goPatterns = Object.fromEntries([...counts.matchAll(/(Pattern\w+): (\d)/g)]
    .map(([, name, n]) => [new RegExp(`${name} +Pattern = "(\\w+)"`).exec(domain)[1], Number(n)]));
  assert.deepEqual(Object.fromEntries(flagPatterns), goPatterns);

  const goEmblems = [...(/var emblems = map\[Emblem\]bool\{([\s\S]*?)\n\}/.exec(domain)[1]).matchAll(/"(\w+)"/g)].map((m) => m[1]);
  assert.deepEqual(flagEmblems.slice(1).sort(), goEmblems.sort());
  for (const [p] of flagPatterns) assert.ok(flagPatternNames[p], p);
  for (const e of flagEmblems) assert.ok(flagEmblemNames[e], e || 'none');
});

test('flagShapes draws a pattern in its colors and the emblem last', () => {
  const shapes = flagShapes({ pattern: 'tricolor_v', colors: [3, 4, 5], emblem: 'star', emblemColor: 7, emblemAt: 'canton' });

  assert.deepEqual(shapes.map((s) => s.color), [3, 4, 5, 7]);
  assert.equal(shapes.at(-1).tag, 'path');
  assert.match(shapes.at(-1).attrs.transform, /^translate\(1\.5 1\.5\)/);
});

test('flagShapes makes a broken flag a plain one', () => {
  assert.deepEqual(flagShapes(null).map((s) => [s.tag, s.color]), [['rect', 0]]);
  assert.deepEqual(flagShapes({ pattern: 'nope', colors: [9] }).map((s) => s.color), [9]);
});

test('fitFlag keeps the colors it can and adds new ones', () => {
  assert.deepEqual(fitFlag({ pattern: 'tricolor_h', colors: [0, 2] }).colors, [0, 2, 1]);
  assert.deepEqual(fitFlag({ pattern: 'plain', colors: [5, 6, 7] }).colors, [5]);
});

test('randomFlag gives different colors of the palette', () => {
  let seed = 7;
  const random = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  for (let i = 0; i < 50; i++) {
    const flag = randomFlag(random);
    assert.equal(new Set(flag.colors).size, flag.colors.length);
    assert.ok(flag.colors.every((c) => c >= 0 && c < COLORS));
  }
});

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

test('stateCenters weighs the provinces of a state by their area', () => {
  const provinces = [{ id: 0, x: 0, y: 0, area: 1 }, { id: 1, x: 10, y: 0, area: 3 }, { id: 2, x: 50, y: 50, area: 9 }];
  const centers = stateCenters([1, 1, 0], provinces);

  assert.equal(centers.size, 1);
  assert.deepEqual({ x: centers.get(1).x, y: centers.get(1).y }, { x: 7.5, y: 0 });
});

// A row of land 0–1–2–3–4 with the sea 5 beside 4.
const line = [0, 1, 2, 3, 4].map((id) => ({ id, links: [id - 1, id + 1].filter((n) => n >= 0 && n <= 5).map((to) => ({ to })) }));
line.push({ id: 5, sea: true, links: [{ to: 4 }] });

test('distances go over land only', () => {
  assert.deepEqual([...distances(line, 0, 10)], [[0, 0], [1, 1], [2, 2], [3, 3], [4, 4]]);
  assert.deepEqual([...distances(line, 2, 1).keys()].sort(), [1, 2, 3]);
});

test('foundable keeps away from capitals and taken land', () => {
  const world = { owners: [1], colonies: [{ province: 4 }], states: [{ id: 's', capital: 0 }], rules: { capitalDistance: 3 } };

  assert.deepEqual(foundable(line, world), [3]);
});

test('colonizable are the free neighbours of the state', () => {
  const world = { owners: [0, 0, 1], colonies: [{ province: 3 }], states: [{ id: 's', capital: 2 }], rules: {} };

  assert.deepEqual(colonizable(line, world, 's'), [1]);
  assert.deepEqual(colonizable(line, world, 'other'), []);
  assert.equal(ownerOf(world, 2).id, 's');
  assert.equal(ownerOf(world, 0), null);
});

test('the offices that colonize and answer applications', () => {
  // The ruler is no office to give.
  const roles = ['ruler', ...worldRoles.map(([r]) => r)];
  assert.ok(!worldRoles.some(([r]) => r === 'ruler'));
  assert.deepEqual(roles.filter(mayColonize), ['ruler', 'minister_economy']);
  assert.deepEqual(roles.filter(mayAnswer), ['ruler', 'minister_interior']);
  assert.ok(governments.length > 0);
});

test('worldPopulation says thousands and millions', () => {
  assert.equal(worldPopulation(20), '20 тыс. чел.');
  assert.equal(worldPopulation(1250), '1,3 млн чел.');
});

test('worldEventParts speaks of a woman as of a woman', () => {
  const e = { kind: 'founded', stateId: 's', name: 'Бореалия', user: { id: 'u', gender: 'female' } };

  assert.deepEqual(worldEventParts(e), [{ person: e.user }, ' основала государство ', { state: 's', name: 'Бореалия' }]);
  assert.deepEqual(worldEventParts({ kind: 'colonized', stateId: 's', name: 'Б', province: 4 })[1], ' заселяет: провинция №4');
  assert.equal(worldEventParts({ kind: 'left', stateId: 's', name: 'Б' })[0], 'Кто-то');
});

test('worldRanking puts the largest first and the equal by name', () => {
  const states = [{ name: 'Б', lands: 2 }, { name: 'А', lands: 2 }, { name: 'В', lands: 5 }];

  assert.deepEqual(worldRanking(states, 'lands').map((s) => s.name), ['В', 'А', 'Б']);
});

test('what the selects of the world offer is written in lower case', () => {
  const texts = [...governments, ...worldRoles].map(([, text]) => text).concat(Object.values(flagPatternNames), Object.values(flagEmblemNames));
  for (const text of texts) assert.equal(text[0], text[0].toLowerCase(), text);
});
