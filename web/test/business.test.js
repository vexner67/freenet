import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';

import {
  firmActionName, firmKindName, firmKinds, firmLine, firmPictures, goodName, goodPictures, goods, hundredths, hundredthsOf, sharePercent, voteOperation,
} from '../static/labels.js';

const catalog = readFileSync(new URL('../../user/internal/user/firm_catalog.go', import.meta.url), 'utf8');
const list = (name) => [...new RegExp(`var ${name} = \\[\\]\\w+\\{([\\s\\S]*?)\\n\\}`).exec(catalog)[1].matchAll(/\b(\w+)\b/g)]
  .map((m) => new RegExp(`\\b${m[1]} +\\w+ = "(\\w+)"`).exec(catalog)[1]);

test('hundredths shows a sum in hundredths of a vote', () => {
  assert.equal(hundredths(170), '1,7');
  assert.equal(hundredths(1205), '12,05');
  assert.equal(hundredths(-4000), '-40');
  assert.equal(hundredths(0), '0');
  assert.equal(hundredths(123456700), '1\u00a0234\u00a0567');
});

test('a sum typed in a field is read as hundredths', () => {
  assert.equal(hundredthsOf('12'), 1200);
  assert.equal(hundredthsOf('1,5'), 150);
  assert.equal(hundredthsOf(' 0.05 '), 5);
  assert.equal(hundredthsOf('2,50'), 250);
  for (const bad of ['', 'x', '1,505', '-1', '1,', ',5', '1 000']) assert.ok(Number.isNaN(hundredthsOf(bad)), bad);
});

test('the enterprises and the goods of the site are those of the user service, with names', () => {
  assert.deepEqual(firmKinds, list('FirmKinds'));
  assert.deepEqual(goods, list('Goods'));
  for (const kind of firmKinds) assert.notEqual(firmKindName(kind), kind, kind);
  for (const good of goods) assert.notEqual(goodName(good), good, good);
});

test('the profile says of an enterprise what it is, its level and the part of the page', () => {
  assert.equal(firmLine({ kind: 'mill', level: 3, shares: 40, total: 100 }), 'Мельница, 3 ур., 40 % акций');
  assert.equal(sharePercent(1, 8), '12,5 %');
  assert.equal(sharePercent(100, 100), '100 %');
  assert.equal(sharePercent(0, 0), '0 %');
});

test('a line of the statement names the action with the enterprises', () => {
  const firm = readFileSync(new URL('../../user/internal/user/firm.go', import.meta.url), 'utf8');
  const service = readFileSync(new URL('../../user/internal/service/firm.go', import.meta.url), 'utf8');
  const notes = [...`${firm}\n${service}`.matchAll(/"(found|deposit|dividend|liquidate|buy_shares|sell_shares)"/g)].map((m) => m[1]);
  for (const note of new Set([...notes, 'refund'])) assert.notEqual(firmActionName(note), firmActionName('?'), note);

  assert.equal(voteOperation({ kind: 'business', amount: -1000, note: 'found' }).title, 'Бизнес: основание предприятия');
  assert.equal(voteOperation({ kind: 'business', amount: 12, note: 'dividend' }).title, 'Бизнес: дивиденды');
  assert.equal(voteOperation({ kind: 'business', amount: 500, note: 'refund' }).title, 'Бизнес: возврат за прежний бизнес');
  assert.equal(voteOperation({ kind: 'business', amount: -500, note: 'kiosk:buy' }).title, 'Бизнес: прежний бизнес');
});

test('every enterprise and every good has its picture', () => {
  const file = (path) => existsSync(new URL(`..${path}`, import.meta.url));
  for (const kind of firmKinds) assert.ok(file(firmPictures[kind]), kind);
  for (const good of goods) assert.ok(file(goodPictures[good]), good);
  assert.ok(file('/static/firms/dividends.svg'));
});
