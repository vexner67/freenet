import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { businessKinds, businessName, hundredths, restockCost, repairCost, businessLevelCost, voteOperation } from '../static/labels.js';

test('hundredths shows a sum in hundredths of a vote', () => {
  assert.equal(hundredths(170), '1,7');
  assert.equal(hundredths(1205), '12,05');
  assert.equal(hundredths(-4000), '-40');
  assert.equal(hundredths(0), '0');
  assert.equal(hundredths(123456700), '1 234 567');
});

test('the costs are counted as the user service counts them', () => {
  assert.equal(restockCost(100, 10), 10);
  assert.equal(restockCost(1, 10), 1);
  assert.equal(repairCost(9850, 1), 2);
  assert.equal(repairCost(10000, 5), 0);
  assert.equal(businessLevelCost(3, 300), 900);
});

test('the businesses of the site are those of the user service', () => {
  const source = readFileSync(new URL('../../user/internal/user/business.go', import.meta.url), 'utf8');
  const kinds = [...source.matchAll(/Business\w+ +BusinessKind = "(\w+)"/g)].map((m) => m[1]);
  assert.deepEqual(businessKinds, kinds);
  for (const kind of kinds) assert.notEqual(businessName(kind), kind);
});

test('a line of the statement names the business and the action', () => {
  assert.equal(voteOperation({ kind: 'business', amount: -500, note: 'kiosk:buy' }).title, 'Бизнес «Ларёк»: покупка');
  assert.equal(voteOperation({ kind: 'business', amount: 12, note: 'cafe:collect' }).title, 'Бизнес «Кафе»: выручка');
});
