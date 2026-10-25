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

test('the picture of a business shows its customers', async () => {
  const { businessWalkers, businessHours, businessCondition } = await import('../static/labels.js');
  const plan = { demand: 10 };

  assert.deepEqual(businessWalkers({ level: 1, customers: 0, served: 0 }, plan), { served: 0, passing: 0 });
  assert.deepEqual(businessWalkers({ level: 1, customers: 10, served: 10 }, plan), { served: 4, passing: 0 });
  assert.deepEqual(businessWalkers({ level: 1, customers: 10, served: 5 }, plan), { served: 2, passing: 2 });
  assert.deepEqual(businessWalkers({ level: 1, customers: 0.5, served: 0 }, plan), { served: 0, passing: 1 });
  assert.deepEqual(businessWalkers({ level: 1, customers: 40, served: 40 }, plan), { served: 6, passing: 0 });
  assert.deepEqual(businessWalkers({ level: 1, customers: 10, served: 10, bankrupt: true }, plan), { served: 0, passing: 0 });

  assert.equal(businessHours(100, 8), 12);
  assert.equal(businessHours(100, 0), null);
  assert.equal(businessCondition(10000), 0);
  assert.equal(businessCondition(6999), 1);
  assert.equal(businessCondition(3999), 2);
});

test('the improvements of the site are those of the user service', async () => {
  const { businessPerks, businessPerkName, businessPerkEffect, businessPerkRate, businessStations } = await import('../static/labels.js');
  const source = readFileSync(new URL('../../user/internal/user/business_perk.go', import.meta.url), 'utf8');
  const rules = readFileSync(new URL('../../user/internal/user/business.go', import.meta.url), 'utf8');
  const perks = [...source.matchAll(/Perk\w+ +BusinessPerk = "(\w+)"/g)].map((m) => m[1]);
  assert.deepEqual(businessPerks.map(([name]) => name), perks);
  for (const [perk, percent] of businessPerks) {
    if (perk !== 'shelves') assert.ok(rules.includes(`perkRate(Perk${perk[0].toUpperCase()}${perk.slice(1)}, ${percent})`), perk);
    assert.notEqual(businessPerkName('cafe', perk), perk);
    assert.notEqual(businessPerkEffect(perk), '');
    assert.equal(businessPerkRate({ perks: [{ perk, tier: 2 }] }, perk), 1 + (2 * percent) / 100);
  }
  assert.equal(businessPerkRate({}, 'decor'), 1);

  assert.equal(businessStations(1), 1);
  assert.equal(businessStations(4), 2);
  assert.equal(businessStations(10), 4);
});
