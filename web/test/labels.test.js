// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { label, relationships, genders, formatBirthday, errorText, formatPostDate, postsCount, wrote, followersCount, followingCount, commentsLink, notificationVerb, unreadCountText, snippet } from '../static/labels.js';

test('relationship agrees with gender', () => {
  assert.equal(label(relationships('female'), 'married'), 'замужем');
  assert.equal(label(relationships('male'), 'married'), 'женат');
  assert.equal(label(relationships(''), 'single'), 'не женат / не замужем');
});

test('unset values have no label, so the profile shows no row', () => {
  assert.equal(label(genders, ''), '');
  assert.equal(label(relationships('female'), ''), '');
});

test('birthday with age in the right form', () => {
  const year = new Date().getFullYear();

  assert.match(formatBirthday(`${year - 21}-01-01`), /^1 января \d{4} г\., 21 год$/);
  assert.match(formatBirthday(`${year - 22}-01-01`), /, 22 года$/);
  assert.match(formatBirthday(`${year - 25}-01-01`), /, 25 лет$/);
  assert.match(formatBirthday(`${year - 11}-01-01`), /, 11 лет$/);
});

test('API errors are translated, unknown ones stay generic', () => {
  assert.equal(errorText(new Error('not a JPEG, PNG, GIF or WebP image')), 'Это не фотография: подойдут JPEG, PNG, GIF и WebP.');
  assert.equal(errorText(new Error('nickname is already taken')), 'Этот никнейм уже занят.');
  assert.equal(errorText(new Error('dial tcp 10.0.0.5')), 'Что-то пошло не так. Попробуйте позже.');
});

test('post dates read like on the old sites', () => {
  const now = new Date(2026, 9, 4, 18, 30); // 4 October 2026, local time

  assert.equal(formatPostDate(new Date(2026, 9, 4, 9, 5).toISOString(), now), 'сегодня в 9:05');
  assert.equal(formatPostDate(new Date(2026, 9, 3, 23, 59).toISOString(), now), 'вчера в 23:59');
  assert.equal(formatPostDate(new Date(2026, 4, 17, 10, 0).toISOString(), now), '17 мая в 10:00');
  assert.equal(formatPostDate(new Date(2025, 11, 31, 8, 7).toISOString(), now), '31 декабря 2025 в 8:07');
});

test('posts count and verb agree', () => {
  assert.equal(postsCount(1), '1 запись');
  assert.equal(postsCount(3), '3 записи');
  assert.equal(postsCount(11), '11 записей');
  assert.equal(postsCount(554), '554 записи');
  assert.equal(wrote('female'), 'написала');
  assert.equal(wrote(''), 'написал');
});

test('follow counters agree with numbers', () => {
  assert.equal(followersCount(1), '1 подписчик');
  assert.equal(followersCount(3), '3 подписчика');
  assert.equal(followersCount(12), '12 подписчиков');
  assert.equal(followingCount(21), '21 подписка');
  assert.equal(followingCount(5), '5 подписок');
});

test('comments link counts, or invites to comment', () => {
  assert.equal(commentsLink(0), 'Комментировать');
  assert.equal(commentsLink(2), 'Комментарии (2)');
  assert.equal(errorText(new Error('fill in your profile before commenting')), 'Сначала заполните свою страницу.');
});

test('notification verbs agree with the gender of who did it', () => {
  assert.equal(notificationVerb('like', 'female'), 'оценила');
  assert.equal(notificationVerb('comment', 'male'), 'оставил');
  assert.equal(notificationVerb('follow', ''), 'подписался(-ась)');
});

test('unread counter', () => {
  assert.equal(unreadCountText(0), '');
  assert.equal(unreadCountText(7), '7');
  assert.equal(unreadCountText(100), '99+');
});

test('snippet cuts long quotes, by characters', () => {
  assert.equal(snippet('коротко', 10), 'коротко');
  assert.equal(snippet('раз два три', 7), 'раз два…');
  assert.equal(snippet('👋👋👋', 2), '👋👋…');
});

test('rate limits ask to wait without promising how long', () => {
  assert.equal(errorText(new Error('too many requests, try again later')), 'Слишком часто. Подождите немного и попробуйте снова.');
});
