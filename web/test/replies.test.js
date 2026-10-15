// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { QUOTE_LENGTH, REACTIONS, quoteSnippet, quoteText, reactionCounts, reacted } from '../static/labels.js';

const bob = { id: 'u2', name: 'Бо', nickname: 'bob' };

test('the reactions of the site are those of the message service', () => {
  const source = readFileSync(new URL('../../message/internal/message/reaction.go', import.meta.url), 'utf8');
  const names = [...source.matchAll(/Reaction\w+\s+Reaction\s*=\s*"([a-z]+)"/g)].map((m) => m[1]);

  assert.deepEqual(REACTIONS.map((r) => r.name), names);
});

test('a quote is one line of the beginning', () => {
  assert.equal(quoteSnippet('  идёшь\n\n завтра?  '), 'идёшь завтра?');

  const long = 'я'.repeat(QUOTE_LENGTH + 5);
  assert.equal(quoteSnippet(long), 'я'.repeat(QUOTE_LENGTH) + '…');
});

test('a quote says what the message answered is', () => {
  assert.equal(quoteText({ id: 'm1', author: bob, text: 'кто берёт [ann|Анна] палатку?' }), 'кто берёт Анна палатку?');
  assert.equal(quoteText({ id: 'm1', author: bob, text: '' }, 'из шифра'), 'из шифра');
  assert.equal(quoteText({ id: 'm1', author: bob, photoCount: 2 }), 'Фотография');
  assert.equal(quoteText({ id: 'm1', author: bob, sticker: 'cat_hi' }), 'Стикер');
  assert.equal(quoteText({ id: 'm1', author: bob, post: true }), 'Запись');
  assert.equal(quoteText({ id: 'm1', author: bob, deleted: true, text: '' }), 'Сообщение удалено');
  assert.equal(quoteText({ id: 'm1', author: null }), 'Сообщение недоступно');
  assert.equal(quoteText({ id: 'm1', author: bob }), 'Сообщение');
});

test('reactions are counted in their order, mine marked', () => {
  const reactions = [{ userId: 'u2', reaction: 'fire' }, { userId: 'u1', reaction: 'heart' }, { userId: 'u3', reaction: 'fire' }];

  assert.deepEqual(reactionCounts(reactions, 'u1').map((r) => [r.name, r.count, r.mine]), [['heart', 1, true], ['fire', 2, false]]);
  assert.deepEqual(reactionCounts([], 'u1'), []);
});

test('a new reaction takes the place of mine', () => {
  const reactions = [{ userId: 'u1', reaction: 'heart' }, { userId: 'u2', reaction: 'fire' }];

  assert.deepEqual(reacted(reactions, 'u1', 'sad'), [{ userId: 'u2', reaction: 'fire' }, { userId: 'u1', reaction: 'sad' }]);
  assert.deepEqual(reacted(reactions, 'u1', ''), [{ userId: 'u2', reaction: 'fire' }]);
  assert.deepEqual(reacted(undefined, 'u1', 'like'), [{ userId: 'u1', reaction: 'like' }]);
});
