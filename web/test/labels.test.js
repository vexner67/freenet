// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { isLongText, selectedCount, partnerPrefix, takesPartner, albumsCount, myAlbumsCount, photosCount, topicsCount, topicMessages, topicMessagesCount, resultsCount, membersCount, myClubsCount, clubsCount, adminsCount, suggestedByEveryone, suggestedByYou, suggestedInClub, audiences, pageReads, onlineReads, onlineCount, presence, wallCount, commonCount, completenessHints, likedBy, adminsDisplays, label, relationships, genders, formatBirthday, errorText, formatPostDate, postsCount, wrote, updatedPhoto, followersCount, followingCount, commentsLink, notificationVerb, unreadCountText, snippet, genitiveName, wallWrites, shareLink, blacklistCount, limitedAccess, messagesWrites, formatDateTime, deactivationReasons, myEventsCount, eventsCount, localTime, inputDate, inputTime } from '../static/labels.js';

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
  assert.equal(updatedPhoto({ gender: 'female' }), 'обновила фотографию на своей странице');
  assert.equal(updatedPhoto({ gender: '' }), 'обновил фотографию на своей странице');
  assert.equal(updatedPhoto({ club: true }), 'обновило фотографию группы');
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
  assert.equal(notificationVerb('like', { gender: 'female' }), 'оценила');
  assert.equal(notificationVerb('comment', { gender: 'male' }), 'оставил');
  assert.equal(notificationVerb('follow', { gender: '' }), 'подписался(-ась)');
  assert.equal(notificationVerb('comment_reply', { gender: 'female' }), 'ответила');
  assert.equal(notificationVerb('comment_like', { gender: '' }), 'оценил(а)');
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
  assert.equal(notificationVerb('wall_post', { gender: 'female' }), 'написала');
  assert.equal(notificationVerb('wall_post', { gender: '' }), 'написал(а)');
  assert.equal(errorText(new Error('you cannot post on this wall')), 'Хозяин страницы не разрешает вам писать у него на стене.');
  assert.deepEqual(wallWrites.map(([value]) => value), ['everyone', 'following', 'only_me']);
});

test('the share link counts reposts, as on openvk', () => {
  assert.equal(shareLink(0), 'Поделиться');
  assert.equal(shareLink(3), 'Поделиться (3)');
  assert.equal(notificationVerb('repost', { gender: 'male' }), 'поделился');
  assert.equal(notificationVerb('repost', { gender: 'female' }), 'поделилась');
  assert.equal(notificationVerb('comment', { club: true, gender: '' }), 'оставило');
  assert.equal(notificationVerb('comment_reply', { club: true }), 'ответило');
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
  assert.equal(myClubsCount(0), 'Вы не состоите ни в одной группе');
  assert.equal(myClubsCount(1), 'Вы состоите в 1 группе');
  assert.equal(myClubsCount(21), 'Вы состоите в 21 группе');
  assert.equal(myClubsCount(5), 'Вы состоите в 5 группах');
  assert.equal(clubsCount(2), '2 группы');
  assert.equal(clubsCount(5), '5 групп');
  assert.equal(errorText(new Error('club address is already taken')), 'Этот адрес уже занят.');
});

test('club admins and who sees the clubs, as openvk says', () => {
  assert.equal(adminsCount(1), '1 администратор');
  assert.equal(adminsCount(3), '3 администратора');
  assert.equal(adminsCount(12), '12 администраторов');
  assert.deepEqual(audiences.map(([value]) => value), ['everyone', 'users', 'following', 'only_me']);
  assert.deepEqual(pageReads, [['everyone', 'Всем желающим'], ['users', 'Пользователям Freenet']]);
  assert.deepEqual(adminsDisplays.map(([value]) => value), ['creator', 'all', 'none']);
  assert.equal(errorText(new Error('the new owner must be an admin of the club')), 'Владельцем можно назначить только администратора группы.');
  assert.equal(errorText(new Error("this user's clubs are hidden")), 'Пользователь скрыл список своих групп.');
  assert.equal(errorText(new Error("this user's follows are hidden")), 'Пользователь скрыл список своих подписок.');
  assert.equal(errorText(new Error('this page is hidden')), 'Настройки приватности этого пользователя не разрешают вам смотреть на его страницу.');
});

test('suggested posts are counted as openvk counts them', () => {
  assert.equal(suggestedByEveryone(1), '1 предложенная запись');
  assert.equal(suggestedByEveryone(3), '3 предложенные записи');
  assert.equal(suggestedByEveryone(5), '5 предложенных записей');
  assert.equal(suggestedByYou(21), '21 предложенная вами запись');
  assert.equal(suggestedByYou(2), '2 предложенные вами записи');
  assert.equal(suggestedInClub(1, false), 'В эту группу предложили 1 запись');
  assert.equal(suggestedInClub(12, true), 'Вы предложили в эту группу 12 записей');
  assert.equal(suggestedInClub(0, false), 'Вы посмотрели всю предложку, поздравляю!');
  assert.equal(errorText(new Error('suggestion not found')), 'Запись уже опубликована или отклонена.');
});

test('the counts of openvk\'s discussions', () => {
  assert.equal(topicsCount(1), '1 тема');
  assert.equal(topicsCount(3), '3 темы');
  assert.equal(topicsCount(0), '0 тем');
  assert.equal(topicMessages(21), '21 сообщение');
  assert.equal(topicMessagesCount(0), 'В теме нет сообщений');
  assert.equal(topicMessagesCount(2), 'В теме 2 сообщения');
  assert.equal(resultsCount(0), 'Ни одного результата');
  assert.equal(resultsCount(5), '5 результатов');
});

test('albums are counted as openvk counts them', () => {
  assert.equal(albumsCount(0), 'Ни одного альбома');
  assert.equal(albumsCount(1), '1 альбом');
  assert.equal(albumsCount(3), '3 альбома');
  assert.equal(albumsCount(11), '11 альбомов');
  assert.equal(myAlbumsCount(0), 'У Вас нет ни одного альбома');
  assert.equal(myAlbumsCount(2), 'У Вас 2 альбома');
  assert.equal(photosCount(1), '1 фотография');
  assert.equal(photosCount(4), '4 фотографии');
  assert.equal(photosCount(25), '25 фотографий');
});

test('"они/их" speak in the plural, as openvk\'s neutral strings', () => {
  assert.equal(wrote('neutral'), 'написали');
  assert.equal(updatedPhoto({ gender: 'neutral' }), 'обновили фотографию на своей странице');
  assert.equal(limitedAccess('neutral'), 'ограничили');
  assert.equal(notificationVerb('like', { gender: 'neutral' }), 'оценили');
  assert.equal(label(genders, 'neutral'), 'они/их');
  assert.equal(label(relationships('neutral'), 'married'), 'в браке');
});

test('a relationship names whom it is with, after openvk\'s prefix', () => {
  assert.equal(takesPartner('engaged'), true);
  assert.equal(takesPartner('looking'), false);
  assert.equal(takesPartner(''), false);
  assert.equal(partnerPrefix('married', 'male'), 'на');
  assert.equal(partnerPrefix('married', 'female'), 'за');
  assert.equal(partnerPrefix('married', ''), 'с');
  assert.equal(partnerPrefix('in_love', 'female'), 'в');
  assert.equal(partnerPrefix('dating', 'male'), 'с');
});

test('a birthday without its year', () => {
  assert.equal(formatBirthday('--05-17'), '17 мая');
});

test('presence is told as openvk tells it', () => {
  const now = new Date(2026, 9, 9, 18, 0);
  assert.equal(presence({ online: true, lastSeenAt: now.toISOString(), gender: 'female' }, now), 'Онлайн');
  assert.equal(presence({ lastSeenAt: new Date(2026, 9, 9, 12, 5).toISOString(), gender: 'female' }, now), 'была в сети сегодня в 12:05');
  assert.equal(presence({ lastSeenAt: new Date(2026, 9, 8, 9, 30).toISOString(), gender: 'neutral' }, now), 'были в сети вчера в 9:30');
  assert.equal(presence({ lastSeenAt: new Date(2026, 8, 1, 9, 30).toISOString() }, now), 'был в сети 1 сентября в 9:30');
  assert.equal(presence({ lastSeenAt: null }, now), '');
  assert.equal(onlineCount(1), '1 человек онлайн');
  assert.equal(onlineCount(3), '3 человека онлайн');
  assert.equal(onlineCount(5), '5 человек онлайн');
  assert.deepEqual(onlineReads.map(([, text]) => text), ['Все', 'Пользователи Freenet', 'Те, на кого я подписан(а)', 'Никто']);
});

test('an album the site keeps refuses changes in words', () => {
  assert.equal(errorText(new Error('this album is kept by the site')), 'Этот альбом сайт ведёт сам: его нельзя изменить.');
});

test('a wall and the follows in common are counted as openvk counts them', () => {
  assert.equal(wallCount(0), 'нет записей');
  assert.equal(wallCount(1), '1 запись');
  assert.equal(wallCount(25), '25 записей');
  assert.equal(commonCount(1), '1 общая подписка');
  assert.equal(commonCount(3), '3 общие подписки');
  assert.equal(commonCount(11), '11 общих подписок');
  assert.deepEqual(Object.keys(completenessHints), ['interests', 'email', 'telegram', 'status']);
});

test('likers are counted as openvk counts them', () => {
  assert.equal(likedBy(0), 'Никому не понравилось');
  assert.equal(likedBy(1), 'Понравилось 1 человеку');
  assert.equal(likedBy(3), 'Понравилось 3 людям');
  assert.equal(likedBy(21), 'Понравилось 21 человеку');
});

test('isLongText is openvk\'s bigText', () => {
  assert.equal(isLongText('привет'), false);
  assert.equal(isLongText('я'.repeat(520)), false);
  assert.equal(isLongText('я'.repeat(521)), true);
  assert.equal(isLongText(Array(10).fill('строка').join('\n')), false);
  assert.equal(isLongText(Array(11).fill('строка').join('\n')), true);
});

test('selectedCount counts the selected messages', () => {
  assert.equal(selectedCount(1), 'Выбрано 1 сообщение');
  assert.equal(selectedCount(3), 'Выбрано 3 сообщения');
  assert.equal(selectedCount(11), 'Выбрано 11 сообщений');
});

test('events are counted as openvk counts them', () => {
  assert.equal(myEventsCount(0), 'Вы не участвуете ни в одной встрече');
  assert.equal(myEventsCount(1), 'Вы участвуете в 1 встрече');
  assert.equal(myEventsCount(5), 'Вы участвуете в 5 встречах');
  assert.equal(eventsCount(0), 'Ни одной встречи');
  assert.equal(eventsCount(1), '1 встреча');
  assert.equal(eventsCount(3), '3 встречи');
  assert.equal(eventsCount(11), '11 встреч');
});

test('the time of an event goes through the inputs and back', () => {
  assert.equal(localTime('', '20:00'), '');
  assert.equal(localTime('2026-12-31', ''), '');
  assert.equal(localTime('когда-нибудь', '20:00'), '');

  const iso = localTime('2026-12-31', '20:05');
  assert.equal(new Date(iso).getTime(), new Date(2026, 11, 31, 20, 5).getTime());
  assert.equal(inputDate(iso), '2026-12-31');
  assert.equal(inputTime(iso), '20:05');
  assert.equal(inputTime(localTime('2027-01-02', '03:04')), '03:04');
});
