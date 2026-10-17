import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

import { notificationVerb } from '../static/labels.js';
import { keyBytes } from '../static/push.js';

// The worker is a classic script: its functions are globals of the context
// it runs in.
function worker(self = {}, globals = {}) {
  const listeners = {};
  const context = { ...globals, self: { ...self, addEventListener: (type, f) => { listeners[type] = f; } } };
  vm.runInNewContext(readFileSync(new URL('../static/sw.js', import.meta.url), 'utf8'), context);
  return { ...context, listeners };
}

// The kinds notification knows, from its domain.
function kinds() {
  const source = readFileSync(new URL('../../notification/internal/notification/notification.go', import.meta.url), 'utf8');
  return [...source.matchAll(/Kind\s*=\s*"([a-z_]+)"/g)].map((m) => m[1]);
}

const ann = { id: 'a1', name: 'Анна Иванова', gender: 'female', nickname: 'ann' };

test('the worker says every kind of notification', () => {
  const { notificationOf } = worker();
  const all = kinds();
  assert.ok(all.length > 20);

  for (const kind of all) {
    const shown = notificationOf({ type: 'notification', kind, id: 'n1', actor: ann });
    assert.ok(shown, kind);
    assert.ok(shown.body.length > 0 && shown.url.startsWith('/'), kind);
    assert.equal(shown.tag, 'n:n1');
  }
});

test('the worker agrees verbs with the gender as the site does', () => {
  const { notificationOf } = worker();

  for (const gender of ['male', 'female', '']) {
    const actor = { ...ann, gender };
    assert.equal(notificationOf({ type: 'notification', kind: 'like', id: 'n1', actor }).body,
      `${notificationVerb('like', actor).replace(/^./, (c) => c.toUpperCase())} вашу запись.`);
    assert.equal(notificationOf({ type: 'notification', kind: 'gift', id: 'n1', actor }).body,
      `${notificationVerb('gift', actor).replace(/^./, (c) => c.toUpperCase())} вам подарок.`);
  }
});

test('the worker leads a message to its dialog and a chat to the chat', () => {
  const { notificationOf } = worker();

  const message = notificationOf({ type: 'message', id: 'm1', actor: ann });
  assert.equal(message.url, '/im/ann');
  assert.equal(message.tag, 'im:a1');
  assert.equal(message.title, 'Анна Иванова');

  const chat = notificationOf({ type: 'chat', id: 'm2', actor: ann, chat: { id: 'c1', title: 'Поход' } });
  assert.equal(chat.url, '/im/chat/c1');
  assert.equal(chat.title, 'Поход');
  assert.equal(chat.body, 'Анна Иванова: новое сообщение.');

  assert.equal(notificationOf({ type: 'chat', id: 'm3', actor: ann, chat: { id: 'c1', title: '' } }).title, 'Беседа');
  assert.equal(notificationOf({ type: 'wat', id: 'x', actor: ann }), null);
  assert.equal(notificationOf({ type: 'notification', kind: 'wat', id: 'x', actor: ann }), null);
});

test('a community is the title of what it did', () => {
  const { notificationOf } = worker();
  const cats = { id: 'c1', name: 'Кошки', club: true, nickname: 'cats' };

  assert.equal(notificationOf({ type: 'notification', kind: 'suggested_post', id: 'n1', actor: cats }).url, '/club/cats/suggested');
  assert.equal(notificationOf({ type: 'notification', kind: 'like', id: 'n1', actor: cats }).body, 'Оценило вашу запись.');
});

test('the worker listens for pushes and clicks', () => {
  const { listeners } = worker();
  for (const type of ['install', 'activate', 'push', 'message', 'notificationclick']) assert.equal(typeof listeners[type], 'function', type);
});

// A phone: no page of the site in front, a shade of notifications, the
// number on the icon and the cache it is kept in.
function phone() {
  const shade = [];
  const badges = [];
  const kept = new Map();
  const cache = {
    match: async (key) => (kept.has(key) ? new Response(kept.get(key)) : undefined),
    put: async (key, response) => { kept.set(key, await response.text()); },
  };
  const self = {
    navigator: { userAgent: 'Mozilla/5.0 (Linux; Android 14) Chrome/130.0', setAppBadge: async (n) => { badges.push(n); } },
    clients: { matchAll: async () => [] },
    registration: {
      getNotifications: async ({ tag } = {}) => shade.filter((n) => !tag || n.tag === tag),
      showNotification: async (title, options) => {
        const at = shade.findIndex((n) => n.tag === options.tag);
        if (at >= 0) shade.splice(at, 1);
        shade.push({ title, ...options });
      },
    },
  };
  return { shade, badges, w: worker(self, { caches: { open: async () => cache }, Response }) };
}

test('a push of something new adds one to the number on the icon', async () => {
  const { shade, badges, w } = phone();
  const wait = (event) => new Promise((resolve) => w.listeners[event.type]({ ...event, waitUntil: resolve }));

  await wait({ type: 'message', data: { unread: 3 } });
  await wait({ type: 'push', data: { json: () => ({ type: 'message', id: 'm1', actor: ann }) } });
  await wait({ type: 'push', data: { json: () => ({ type: 'message', id: 'm2', actor: ann }) } });
  await wait({ type: 'push', data: { json: () => ({ type: 'notification', kind: 'follow', id: 'n1', actor: ann }) } });

  // The second message is of a dialog already in the shade.
  assert.deepEqual(badges, [4, 5]);
  assert.equal(shade.length, 2);
  assert.ok(shade.every((n) => n.badge === '/static/badge.png'));
});

test('the badge of Android is a file of the site', () => {
  assert.ok(readFileSync(new URL('../static/badge.png', import.meta.url)).length > 0);
});

test('keyBytes reads the base64url key of the server', () => {
  assert.deepEqual([...keyBytes('BAEC_-8')], [4, 1, 2, 255, 239]);
  assert.deepEqual([...keyBytes('AQ')], [1]);
});
