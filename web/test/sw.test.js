import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';

import { notificationVerb } from '../static/labels.js';
import { keyBytes } from '../static/push.js';

// The worker is a classic script: its functions are globals of the context
// it runs in.
function worker() {
  const listeners = {};
  const context = { self: { addEventListener: (type, f) => { listeners[type] = f; } } };
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
  assert.equal(notificationOf({ type: 'notification', kind: 'event_reminder', id: 'n1', actor: cats }).url, '/club/cats');
  assert.equal(notificationOf({ type: 'notification', kind: 'like', id: 'n1', actor: cats }).body, 'Оценило вашу запись.');
});

test('the worker listens for pushes and clicks', () => {
  const { listeners } = worker();
  for (const type of ['install', 'activate', 'push', 'notificationclick']) assert.equal(typeof listeners[type], 'function', type);
});

test('keyBytes reads the base64url key of the server', () => {
  assert.deepEqual([...keyBytes('BAEC_-8')], [4, 1, 2, 255, 239]);
  assert.deepEqual([...keyBytes('AQ')], [1]);
});
