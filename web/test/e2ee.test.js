import { test, beforeEach } from 'node:test';
import assert from 'node:assert/strict';

// The browser's storage, kept in memory.
const stored = new Map();
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: {
    getItem: (k) => (stored.has(k) ? stored.get(k) : null),
    setItem: (k, v) => stored.set(k, String(v)),
    removeItem: (k) => stored.delete(k),
  },
});

const { api, ApiError } = await import('../static/api.js');
const e2ee = await import('../static/e2ee.js');

// The server: the key of each user, as PUT; whoever calls is `caller`.
let server;
let caller;
let sent;

beforeEach(() => {
  stored.clear();
  server = new Map();
  caller = 'ann';
  sent = [];

  api.myMessageKey = async () => ({ publicKey: server.get(caller) ?? null });
  api.setMessageKey = async (key) => { server.set(caller, key); };
  api.messageKey = async (nickname) => ({ publicKey: server.get(nickname) ?? null });
  api.sendMessage = async (nickname, body) => {
    sent.push({ nickname, ...body });
    return { id: 'm9', out: true, sealed: body.sealed, forwarded: [], forwardedCount: body.forward.length };
  };
});

// as runs fn as user: the browser keeps keys per user, the server answers
// for the caller.
async function as(user, fn) {
  const before = caller;

  caller = user;
  try {
    return await fn();
  } finally {
    caller = before;
  }
}

test('a user without a key gets one made and set up by the browser', async () => {
  assert.equal(await e2ee.ensureKey('ann'), e2ee.KEY_READY);
  assert.equal(e2ee.fromBase64(server.get('ann')).length, 32);
  assert.equal(e2ee.ownKey('ann'), server.get('ann'));

  // Asked again soon, it does not ask the server.
  api.myMessageKey = async () => assert.fail('asked the server');
  assert.equal(await e2ee.ensureKey('ann'), e2ee.KEY_READY);
});

test('a key set up elsewhere is missing here, or this one is stale', async () => {
  server.set('ann', e2ee.toBase64(new Uint8Array(32).fill(7)));
  assert.equal(await e2ee.ensureKey('ann'), e2ee.KEY_MISSING);
  assert.equal(e2ee.hasKeys('ann'), false);

  server.delete('ann');
  await e2ee.ensureKey('ann', { force: true });
  server.set('ann', e2ee.toBase64(new Uint8Array(32).fill(7)));
  assert.equal(await e2ee.ensureKey('ann', { force: true }), e2ee.KEY_STALE);
});

test('a message sealed for two opens for both and no one else', async () => {
  await as('ann', () => e2ee.ensureKey('ann'));
  await as('bob', () => e2ee.ensureKey('bob'));
  await as('cat', () => e2ee.ensureKey('cat'));

  const payload = { t: 'привет 👋\nвторая строка', f: ['из пересланного', ''], r: 'на что ответ' };
  const sealed = await e2ee.seal('ann', server.get('bob'), payload);
  const bytes = e2ee.fromBase64(sealed);

  // The format the server checks: version, the sender's and the
  // recipient's keys, the IV, the ciphertext with its tag.
  assert.equal(bytes[0], 1);
  assert.equal(e2ee.toBase64(bytes.subarray(1, 33)), server.get('ann'));
  assert.equal(e2ee.toBase64(bytes.subarray(33, 65)), server.get('bob'));
  assert.ok(bytes.length >= 93);

  assert.deepEqual(await e2ee.open('bob', sealed), payload);
  assert.deepEqual(await e2ee.open('ann', sealed), payload);
  assert.equal(await e2ee.open('cat', sealed), null);

  // A byte changed anywhere, the header too, and it does not open.
  for (const at of [5, 70, bytes.length - 1]) {
    const broken = bytes.slice();
    broken[at] ^= 1;
    assert.equal(await e2ee.open('bob', e2ee.toBase64(broken)), null, `byte ${at}`);
  }
});

test('an older key still opens what it sealed', async () => {
  await as('ann', () => e2ee.ensureKey('ann'));
  await as('bob', () => e2ee.ensureKey('bob'));

  const old = await e2ee.seal('ann', server.get('bob'), { t: 'старое', f: [] });
  await as('bob', () => e2ee.newKey('bob'));

  assert.equal((await e2ee.open('bob', old)).t, 'старое');
  assert.equal((await e2ee.open('ann', old)).t, 'старое');
});

test('openMessages puts what messages say in them, and marks those it cannot open', async () => {
  await as('ann', () => e2ee.ensureKey('ann'));
  await as('bob', () => e2ee.ensureKey('bob'));

  const sealed = await e2ee.seal('bob', server.get('ann'), { t: 'вот', f: ['первое', 'скрытое', 'третье'] });
  const messages = [
    { id: 'm1', sealed, forwardedCount: 3, forwarded: [{ index: 0, text: '' }, { index: 2, text: '' }] },
    { id: 'm2', sealed: null, photos: [{}] },
    { id: 'm3', sealed: e2ee.toBase64(new Uint8Array(100)) },
  ];

  await e2ee.openMessages('ann', messages);

  assert.equal(messages[0].text, 'вот');
  assert.deepEqual(messages[0].forwarded.map((f) => f.text), ['первое', 'третье']);
  assert.deepEqual(messages[0].words, ['первое', 'скрытое', 'третье']);
  assert.equal(messages[0].unreadable, false);
  assert.equal(messages[1].text, '');
  assert.equal(messages[1].unreadable, false);
  assert.equal(messages[2].text, '');
  assert.equal(messages[2].unreadable, true);
});

test('carriedWords are in the order the server keeps forwarded messages', async () => {
  const dialog = { id: '0002', text: 'из диалога', forwardedCount: 2, words: ['под ним', 'ещё'] };
  const chat = { id: '0001', chatId: 'c1', text: 'из беседы', forwardedCount: 3, forwarded: [{ index: 0, text: 'а' }, { index: 2, text: 'в' }] };

  assert.deepEqual(await e2ee.carriedWords([dialog, chat]), ['из беседы', 'а', '', 'в', 'из диалога', 'под ним', 'ещё']);
  await assert.rejects(e2ee.carriedWords([{ id: '1', unreadable: true }]), /не расшифровано/);

  // A chat message the page has the beginning of is read whole.
  api.chatMessage = async (id) => ({ id, chatId: 'c1', text: 'целиком', forwardedCount: 0 });
  assert.deepEqual(await e2ee.carriedWords([{ id: '7', chatId: 'c1', text: 'цел', textCut: true }]), ['целиком']);
});

test('sendSealed seals for both and, when a key changed meanwhile, once more', async () => {
  await as('ann', () => e2ee.ensureKey('ann'));
  await as('bob', () => e2ee.ensureKey('bob'));

  const m = await e2ee.sendSealed('ann', 'bob', 'привет', { photos: ['ph1'] });
  assert.equal(m.text, 'привет');
  assert.equal((await e2ee.open('bob', sent[0].sealed)).t, 'привет');
  assert.deepEqual(sent[0].photos, ['ph1']);

  // Photos alone say nothing: nothing is sealed.
  await e2ee.sendSealed('ann', 'bob', '', { photos: ['ph2'] });
  assert.equal(sent[1].sealed, null);

  // Bob set up a new key as the message went: the server refuses it
  // once, the page asks again and seals for the keys as they are.
  let refused = 0;
  const send = api.sendMessage;
  api.sendMessage = async (nickname, body) => {
    if (refused++ === 0) {
      await as('bob', () => e2ee.newKey('bob'));
      throw new ApiError(409, 'conflict', 'the encryption keys changed');
    }
    return send(nickname, body);
  };
  await e2ee.sendSealed('ann', 'bob', 'ещё раз');
  assert.equal(refused, 2);
  const again = e2ee.fromBase64(sent.at(-1).sealed);
  assert.equal(e2ee.toBase64(again.subarray(33, 65)), server.get('bob'));
  assert.equal((await e2ee.open('bob', sent.at(-1).sealed)).t, 'ещё раз');

  // Refused twice, it gives up.
  api.sendMessage = async () => { throw new ApiError(409, 'conflict', 'the encryption keys changed'); };
  await assert.rejects(e2ee.sendSealed('ann', 'bob', 'нет'), /keys changed/);
  api.sendMessage = send;

  // An answer quotes the words it answers, sealed: the server cannot.
  const answer = await e2ee.sendSealed('ann', 'bob', '', { photos: ['ph3'], replyTo: { id: 'm1', text: 'идёшь\n  завтра?' } });
  assert.equal(sent.at(-1).replyTo, 'm1');
  assert.equal(answer.quoted, 'идёшь завтра?');
  assert.equal((await e2ee.open('bob', sent.at(-1).sealed)).r, 'идёшь завтра?');

  // Nobody can write to one with no key.
  await assert.rejects(e2ee.sendSealed('ann', 'cat', 'эй'), (e) => e === e2ee.NO_PEER_KEY);
});

test('keys move to another browser in a file locked with a code', async () => {
  await as('ann', () => e2ee.ensureKey('ann'));
  const sealed = await e2ee.seal('ann', server.get('ann'), { t: 'себе', f: [] });

  const code = e2ee.newCode();
  assert.match(code, /^[2-9A-HJKMNP-Z]{4}(-[2-9A-HJKMNP-Z]{4}){4}$/);
  const file = await e2ee.exportKeys('ann', code);

  // Another browser: nothing kept.
  e2ee.forgetKeys('ann');
  assert.equal(await e2ee.ensureKey('ann', { force: true }), e2ee.KEY_MISSING);
  assert.equal(await e2ee.open('ann', sealed), null);

  await assert.rejects(e2ee.importKeys('ann', file, 'AAAA-AAAA-AAAA-AAAA-AAAA'), /код/);
  await assert.rejects(e2ee.importKeys('bob', file, code), /другой страницы/);
  await assert.rejects(e2ee.importKeys('ann', 'not a file', code), /код/);

  // Typed in any case, without dashes.
  assert.equal(await e2ee.importKeys('ann', file, code.toLowerCase().replaceAll('-', ' ')), e2ee.KEY_READY);
  assert.equal((await e2ee.open('ann', sealed)).t, 'себе');
});

test('the safety code is the same on both sides', async () => {
  const a = e2ee.toBase64(new Uint8Array(32).fill(1));
  const b = e2ee.toBase64(new Uint8Array(32).fill(2));

  assert.equal(await e2ee.safetyCode(a, b), await e2ee.safetyCode(b, a));
  assert.match(await e2ee.safetyCode(a, b), /^\d{4}( \d{4}){4}$/);
  assert.notEqual(await e2ee.safetyCode(a, b), await e2ee.safetyCode(a, a));
});

test('a peer key is remembered as first seen', () => {
  assert.equal(e2ee.notePeerKey('ann', 'u2', 'k1'), 'new');
  assert.equal(e2ee.notePeerKey('ann', 'u2', 'k1'), 'same');
  assert.equal(e2ee.notePeerKey('ann', 'u2', 'k2'), 'changed');
  assert.equal(e2ee.notePeerKey('ann', 'u2', 'k2'), 'same');
});
