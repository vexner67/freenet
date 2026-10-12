import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MAX_PARKED, accountName, accountOf, expiresAt, isDue, kept, left, swapped, tended } from '../static/accounts.js';

const account = (id) => ({ id, name: id, surname: '', nickname: id, accessToken: 'a-' + id, refreshToken: 'r-' + id });

test('accountOf keeps the session and whose it is, nothing else', () => {
  const me = { id: 'u1', name: 'Анна', surname: 'Иванова', nickname: 'ann', email: 'ann@example.com', avatar: { small: '/media/a.jpg', large: '/media/b.jpg' } };

  assert.deepEqual(accountOf(me, { accessToken: 'a', refreshToken: 'r', extra: 1 }),
    { id: 'u1', name: 'Анна', surname: 'Иванова', nickname: 'ann', avatar: '/media/a.jpg', accessToken: 'a', refreshToken: 'r' });
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

// token is an access token that ends at exp, in seconds.
const token = (exp) => `h.${btoa(JSON.stringify({ exp })).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')}.s`;

test('expiresAt reads when an access token ends', () => {
  assert.equal(expiresAt(token(1700000000)), 1700000000000);
  assert.equal(expiresAt('not a token'), 0);
  assert.equal(expiresAt(''), 0);
});

test('isDue: a parked account is looked after now and then, and before its token ends', () => {
  const now = 1700000000000;
  const fresh = token(now / 1000 + 600);

  assert.equal(isDue({ accessToken: fresh }, now), true);
  assert.equal(isDue({ accessToken: fresh, tendedAt: now - 30000 }, now), false);
  assert.equal(isDue({ accessToken: fresh, tendedAt: now - 5 * 60000 }, now), true);
  assert.equal(isDue({ accessToken: token(now / 1000 + 30), tendedAt: now - 30000 }, now), true);
  assert.equal(isDue({ accessToken: token(now / 1000 - 30), tendedAt: now - 30000 }, now), true);
});

test('tended puts what was learnt into the account, and brings none back', () => {
  const parked = [account('a'), account('b')];

  assert.equal(tended(parked, 'b', { unread: 3 })[1].unread, 3);
  assert.equal(tended(parked, 'b', { unread: 3 })[0].unread, undefined);
  assert.deepEqual(tended(parked, 'gone', { unread: 3 }), parked);
});
