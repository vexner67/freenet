import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MAX_PARKED, accountName, accountOf, kept, left, swapped } from '../static/accounts.js';

const account = (id) => ({ id, name: id, surname: '', nickname: id, accessToken: 'a-' + id, refreshToken: 'r-' + id });

test('accountOf keeps the session and whose it is, nothing else', () => {
  const me = { id: 'u1', name: 'Анна', surname: 'Иванова', nickname: 'ann', email: 'ann@example.com' };

  assert.deepEqual(accountOf(me, { accessToken: 'a', refreshToken: 'r', extra: 1 }),
    { id: 'u1', name: 'Анна', surname: 'Иванова', nickname: 'ann', accessToken: 'a', refreshToken: 'r' });
  assert.equal(accountName(accountOf(me, {})), 'Анна Иванова');
  assert.equal(accountName(accountOf({ id: 'u2' }, {})), 'Новая страница');
});

test('kept puts an account first, once', () => {
  assert.deepEqual(kept([account('a'), account('b')], account('b')).map((a) => a.id), ['b', 'a']);
  assert.deepEqual(kept([], account('a')).map((a) => a.id), ['a']);
  assert.deepEqual(left([account('a'), account('b')], 'a').map((a) => a.id), ['b']);

  const many = Array.from({ length: MAX_PARKED + 3 }, (_, i) => account('u' + i));
  assert.equal(kept(many, account('new')).length, MAX_PARKED + 1);
});

test('swapped signs in as a parked account and sets the current one aside', () => {
  const parked = [account('a'), account('b')];

  const result = swapped(parked, 'b', account('me'));
  assert.deepEqual(result.tokens, { accessToken: 'a-b', refreshToken: 'r-b' });
  assert.deepEqual(result.parked.map((a) => a.id), ['me', 'a']);

  // From the page of signing in no one is set aside.
  assert.deepEqual(swapped(parked, 'a', null).parked.map((a) => a.id), ['b']);
  assert.equal(swapped(parked, 'nobody', account('me')), null);
});
