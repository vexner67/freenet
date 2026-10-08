// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { membersCount, label, relationships, genders, formatBirthday, errorText, formatPostDate, postsCount, wrote, followersCount, followingCount, commentsLink, notificationVerb, unreadCountText, snippet, genitiveName, wallWrites, shareLink, blacklistCount, limitedAccess, messagesWrites, formatDateTime, deactivationReasons } from '../static/labels.js';

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
  assert.equal(errorText(new Error('the post can no longer be edited')), 'Запись можно редактировать только в течение недели после публикации.');
  assert.equal(errorText(new Error('the message can no longer be deleted')), 'Сообщение можно удалить только в течение суток после отправки.');
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
  assert.equal(notificationVerb('comment_reply', 'female'), 'ответила');
  assert.equal(notificationVerb('comment_like', ''), 'оценил(а)');
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

test('names in the genitive, for "на стене Петра Петрова"', () => {
  const cases = [
    [{ name: 'Пётр', surname: 'Петров', gender: 'male' }, 'Петра Петрова'],
    [{ name: 'Иван', surname: 'Иванов' }, 'Ивана Иванова'],
    [{ name: 'Анна', surname: 'Петрова', gender: 'female' }, 'Анны Петровой'],
    [{ name: 'Ольга', surname: 'Ильина' }, 'Ольги Ильиной'],
    [{ name: 'Мария', surname: 'Чайковская', gender: 'female' }, 'Марии Чайковской'],
    [{ name: 'Андрей', surname: 'Достоевский', gender: 'male' }, 'Андрея Достоевского'],
    [{ name: 'Лев', surname: 'Толстой', gender: 'male' }, 'Льва Толстого'],
    [{ name: 'Игорь', surname: 'Гоголь', gender: 'male' }, 'Игоря Гоголя'],
    [{ name: 'Любовь', surname: 'Шевчук', gender: 'female' }, 'Любови Шевчук'],
    [{ name: 'Никита', surname: 'Шевченко', gender: 'male' }, 'Никиты Шевченко'],
    [{ name: 'Илья', surname: 'Черных', gender: 'male' }, 'Ильи Черных'],
    [{ name: 'Павел', surname: 'Шевчук', gender: 'male' }, 'Павла Шевчука'],
    [{ name: 'Наташа', surname: '', gender: 'female' }, 'Наташи'],
    [{ name: 'ИВАН', surname: 'ИВАНОВ' }, 'ИВАНА ИВАНОВА'],
    [{ name: 'John', surname: 'Smith', gender: 'male' }, 'John Smith'],
    [{ name: 'Кармен', surname: 'Electra', gender: 'female' }, 'Кармен Electra'],
  ];

  for (const [person, want] of cases) {
    assert.equal(genitiveName(person), want, JSON.stringify(person));
  }
});

test('a wall post notification and its error read like openvk', () => {
  assert.equal(notificationVerb('wall_post', 'female'), 'написала');
  assert.equal(notificationVerb('wall_post', ''), 'написал(а)');
  assert.equal(errorText(new Error('you cannot post on this wall')), 'Хозяин страницы не разрешает вам писать у него на стене.');
  assert.deepEqual(wallWrites.map(([value]) => value), ['everyone', 'following', 'only_me']);
});

test('the share link counts reposts, as on openvk', () => {
  assert.equal(shareLink(0), 'Поделиться');
  assert.equal(shareLink(3), 'Поделиться (3)');
  assert.equal(notificationVerb('repost', 'male'), 'поделился');
  assert.equal(notificationVerb('repost', 'female'), 'поделилась');
});

test('blacklist and messages privacy read like openvk', () => {
  assert.equal(blacklistCount(0), 'В вашем чёрном списке ещё нет пользователей.');
  assert.equal(blacklistCount(1), 'В вашем чёрном списке 1 пользователь.');
  assert.equal(blacklistCount(3), 'В вашем чёрном списке 3 пользователя.');
  assert.equal(blacklistCount(11), 'В вашем чёрном списке 11 пользователей.');
  assert.equal(limitedAccess('female'), 'ограничила');
  assert.equal(limitedAccess(''), 'ограничил(а)');
  assert.deepEqual(messagesWrites.map(([value]) => value), ['everyone', 'following', 'nobody']);
  assert.match(errorText(new Error('you cannot write to this user')), /ограничил круг лиц/);
});

test('formatDateTime is openvk\'s date of a deleted page', () => {
  assert.equal(formatDateTime(new Date(2027, 4, 7, 12, 5).toISOString()), '7 мая 2027 в 12:05');
});

test('deactivationReasons are openvk\'s, the last one without words', () => {
  assert.equal(deactivationReasons.length, 6);
  assert.equal(deactivationReasons.at(-1)[1], '');
  assert.ok(deactivationReasons.slice(0, -1).every(([title, text]) => title && text));
});

test('errorText knows deleted pages', () => {
  assert.equal(errorText({ message: 'this page is deleted' }), 'Страница пользователя заблокирована или удалена.');
  assert.equal(errorText({ message: 'this page can no longer be restored' }), 'Срок восстановления страницы истёк.');
});

test('clubs', () => {
  assert.equal(membersCount(0), 'Ни одного участника');
  assert.equal(membersCount(1), '1 участник');
  assert.equal(membersCount(3), '3 участника');
  assert.equal(membersCount(11), '11 участников');
  assert.equal(errorText(new Error('club address is already taken')), 'Этот адрес уже занят.');
});
