import test from 'node:test';
import assert from 'node:assert/strict';

import { POKER_PAID, handsCount, pokerCard, pokerHandName, pokerOutcome, pokerPay } from '../static/labels.js';

test('a card is its rank, its suit and its colour', () => {
  assert.deepEqual(pokerCard('As'), { rank: 'A', suit: '♠', red: false });
  assert.deepEqual(pokerCard('Td'), { rank: '10', suit: '♦', red: true });
  assert.deepEqual(pokerCard('2h'), { rank: '2', suit: '♥', red: true });
  assert.equal(pokerCard('1s'), null);
  assert.equal(pokerCard('back'), null);
  assert.equal(pokerCard(undefined), null);
});

test('the ante pays by the table from a royal flush down', () => {
  const pays = [100, 20, 10, 3, 2, 1];

  assert.equal(POKER_PAID.length, pays.length);
  assert.equal(pokerPay('royal_flush', pays), 100);
  assert.equal(pokerPay('straight_flush', pays), 20);
  assert.equal(pokerPay('four', pays), 10);
  assert.equal(pokerPay('full_house', pays), 3);
  assert.equal(pokerPay('flush', pays), 2);
  assert.equal(pokerPay('straight', pays), 1);
  assert.equal(pokerPay('pair', [100, 20, 10, 3, 2, 5]), 5);
});

test('hands are named and counted', () => {
  assert.equal(pokerHandName('two_pair'), 'Две пары');
  assert.equal(pokerHandName('mystery'), 'mystery');
  assert.equal(handsCount(1), '1 раздача');
  assert.equal(handsCount(3), '3 раздачи');
  assert.equal(handsCount(5), '5 раздач');
});

test('a hand over is told in words', () => {
  const hand = { ante: 10, payout: 0, playerHand: 'flush', dealerHand: 'pair' };

  assert.equal(pokerOutcome({ ...hand, state: 'folded' }), 'Вы сбросили карты: анте 10 голосов проиграно.');
  assert.equal(pokerOutcome({ ...hand, state: 'won', payout: 50 }), 'У вас флеш, у дилера пара. Вы выиграли 50 голосов!');
  assert.equal(pokerOutcome({ ...hand, state: 'tied', payout: 30, dealerHand: 'flush' }), 'Ничья: флеш у обоих. Ставки вернулись — 30 голосов.');
  assert.equal(pokerOutcome({ ...hand, state: 'lost', playerHand: 'pair', dealerHand: 'flush' }), 'У дилера флеш, у вас пара. Ставки проиграны.');
  assert.match(pokerOutcome({ ...hand, state: 'unqualified', payout: 40, dealerHand: 'high_card' }), /^У дилера старшая карта — он не играет/);
  assert.equal(pokerOutcome({ ...hand, state: 'dealt' }), '');
});
