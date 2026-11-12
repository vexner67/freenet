import test from 'node:test';
import assert from 'node:assert/strict';
import { voteOperation } from '../static/labels.js';

test('the statement keeps the history of enterprises after closing the games', () => {
  assert.equal(voteOperation({ kind: 'business', amount: -1000, note: 'found' }).title, 'Бизнес: основание предприятия');
  assert.equal(voteOperation({ kind: 'business', amount: 12, note: 'dividend' }).title, 'Бизнес: дивиденды');
  assert.equal(voteOperation({ kind: 'business', amount: 500, note: 'refund' }).title, 'Бизнес: возврат за прежний бизнес');
  assert.equal(voteOperation({ kind: 'business', amount: -500, note: 'kiosk:buy' }).title, 'Бизнес: прежний бизнес');
  assert.equal(voteOperation({ kind: 'closing', amount: 500, note: 'firm' }).title, 'Закрытие игр');
});
