import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import {
  businessCondition, businessKinds, businessName, businessStations, businessWalkers, hundredths, offerName, orderCost,
  professionName, supplierName, supplyName, traitName, voteOperation,
} from '../static/labels.js';

const catalog = readFileSync(new URL('../../user/internal/user/business_catalog.go', import.meta.url), 'utf8');
const constants = (source, type) => [...source.matchAll(new RegExp(`\\w+ +${type} = "(\\w+)"`, 'g'))].map((m) => m[1]);

test('hundredths shows a sum in hundredths of a vote', () => {
  assert.equal(hundredths(170), '1,7');
  assert.equal(hundredths(1205), '12,05');
  assert.equal(hundredths(-4000), '-40');
  assert.equal(hundredths(0), '0');
  assert.equal(hundredths(123456700), '1 234 567');
});

test('an order costs what the user service counts', () => {
  assert.equal(orderCost(100, 10), 10);
  assert.equal(orderCost(1, 10), 1);
  assert.equal(orderCost(30, 33.333333333333336), 10);
  assert.equal(orderCost(0, 50), 0);
});

test('the businesses of the site are those of the user service', () => {
  const source = readFileSync(new URL('../../user/internal/user/business.go', import.meta.url), 'utf8');
  const kinds = constants(source, 'BusinessKind');
  assert.deepEqual(businessKinds, kinds);
  for (const kind of kinds) assert.notEqual(businessName(kind), kind);
});

test('the supplies, offers, professions and suppliers of the catalog have names', () => {
  for (const supply of constants(catalog, 'Supply')) assert.notEqual(supplyName(supply), supply, supply);
  for (const offer of constants(catalog, 'Offer')) assert.notEqual(offerName(offer), offer, offer);
  for (const profession of constants(catalog, 'Profession')) assert.notEqual(professionName(profession), profession, profession);
  for (const supplier of constants(catalog, 'Supplier')) assert.notEqual(supplierName(supplier), supplier, supplier);

  const staff = readFileSync(new URL('../../user/internal/user/business_staff.go', import.meta.url), 'utf8');
  for (const trait of constants(staff, 'Trait').filter(Boolean)) assert.notEqual(traitName(trait), trait, trait);
});

test('a line of the statement names the business and the action', () => {
  assert.equal(voteOperation({ kind: 'business', amount: -500, note: 'kiosk:buy' }).title, 'Бизнес «Ларёк»: покупка');
  assert.equal(voteOperation({ kind: 'business', amount: 12, note: 'cafe:collect' }).title, 'Бизнес «Кафе»: выручка');
});

test('the picture of a business shows its customers', () => {
  const spec = { offers: [{ demand: 6 }, { demand: 4 }] };
  const business = (demand, served, more = {}) => ({ level: 1, offers: [{ demand, served }], workers: [], supplies: [], ...more });

  assert.deepEqual(businessWalkers(business(0, 0), spec), { served: 0, passing: 0 });
  assert.deepEqual(businessWalkers(business(10, 10), spec), { served: 4, passing: 0 });
  assert.deepEqual(businessWalkers(business(10, 5), spec), { served: 2, passing: 2 });
  assert.deepEqual(businessWalkers(business(0.5, 0), spec), { served: 0, passing: 1 });
  assert.deepEqual(businessWalkers(business(40, 40), spec), { served: 6, passing: 0 });
  assert.deepEqual(businessWalkers(business(10, 10, { bankrupt: true }), spec), { served: 0, passing: 0 });

  assert.equal(businessCondition(10000), 0);
  assert.equal(businessCondition(6999), 1);
  assert.equal(businessCondition(3999), 2);
  assert.equal(businessStations(1), 1);
  assert.equal(businessStations(4), 2);
  assert.equal(businessStations(10), 4);
});
