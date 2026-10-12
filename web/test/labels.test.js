// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { worldviews, politicalViews, forwardTree, forwardedCount, messagePreview, mentionParts, plainMentions, replyMention, isLongText, selectedCount, partnerPrefix, takesPartner, albumsCount, myAlbumsCount, photosCount, topicsCount, topicMessages, topicMessagesCount, resultsCount, membersCount, myClubsCount, clubsCount, adminsCount, suggestedByEveryone, suggestedByYou, suggestedInClub, audiences, pageReads, onlineReads, onlineCount, presence, wallCount, commonCount, completenessHints, likedBy, sharedBy, adminsDisplays, label, relationships, genders, formatBirthday, errorText, formatPostDate, postsCount, wrote, updatedPhoto, followersCount, followingCount, commentsLink, notificationVerb, unreadCountText, snippet, foundParts, foundSnippet, foundText, genitiveName, wallWrites, shareLink, blacklistCount, sessionsCount, deviceName, limitedAccess, messagesWrites, formatDateTime, deactivationReasons, myEventsCount, eventsCount, pastEventsCount, localTime, inputDate, inputTime, aboutStatistics, avatarStyles, mainPages, menuItems, themes, ratings, nsfwContents, postViews, timezones, setTimezone, archiveYears, pollVoters, pollMeta, pollPercent, pollAsksVote, daysCount, notesCount, foundNotes, favesCount, feedSettings, feedPageSizes, ignoreLabel, chatTitle, defaultChatTitle, chatMembers, chatAction, chatActionParts, chatPreview, chatStateText, accusativeName, chatsInvites } from '../static/labels.js';

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
  assert.deepEqual(pageReads, [['everyone', 'всем желающим'], ['users', 'пользователям Freenet']]);
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

test('notes are counted and found as openvk says', () => {
  assert.equal(notesCount(0), 'Ни одной заметки');
  assert.equal(notesCount(1), '1 заметка');
  assert.equal(notesCount(3), '3 заметки');
  assert.equal(notesCount(12), '12 заметок');
  assert.equal(foundNotes(0), 'Ни одной заметки');
  assert.equal(foundNotes(1), 'Найдена 1 заметка');
  assert.equal(foundNotes(21), 'Найдена 21 заметка');
  assert.equal(foundNotes(4), 'Найдено 4 заметки');
  assert.equal(foundNotes(7), 'Найдено 7 заметок');
  assert.equal(errorText({ message: "this user's notes are hidden" }), 'Пользователь скрыл свои заметки.');
  assert.ok(menuItems.some(([item, text]) => item === 'notes' && text === 'Заметки'));
});

test('faves are counted as openvk says', () => {
  assert.equal(favesCount(0), 'Ни одной закладки');
  assert.equal(favesCount(1), '1 закладка');
  assert.equal(favesCount(22), '22 закладки');
  assert.equal(favesCount(5), '5 закладок');
  assert.equal(favesCount(11), '11 закладок');
});

test('there are no pronouns: a gender is male, female or not set', () => {
  assert.deepEqual(genders, [['', 'не указан'], ['male', 'мужской'], ['female', 'женский']]);

  // openvk's "они/их", chosen before it was taken away, reads as not set.
  assert.equal(wrote('neutral'), 'написал');
  assert.equal(updatedPhoto({ gender: 'neutral' }), 'обновил фотографию на своей странице');
  assert.equal(limitedAccess('neutral'), 'ограничил(а)');
  assert.equal(notificationVerb('like', { gender: 'neutral' }), 'оценил(а)');
  assert.equal(label(genders, 'neutral'), '');
  assert.equal(label(relationships('neutral'), 'married'), 'в браке');
});

test('what a select offers is written in lower case', () => {
  const lists = [genders, worldviews, politicalViews, wallWrites, messagesWrites, chatsInvites, audiences, pageReads, onlineReads, avatarStyles, themes, ratings, nsfwContents, postViews, mainPages];

  for (const [, text] of lists.flat()) assert.equal(text[0], text[0].toLowerCase(), text);

  assert.equal(label(politicalViews, 'moderate'), '');
  assert.equal(label(politicalViews, 'moderate_islamism'), 'умеренный исламизм');
  assert.equal(label(politicalViews, 'radical_islamism'), 'радикальный исламизм');
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
  assert.equal(presence({ lastSeenAt: new Date(2026, 9, 8, 9, 30).toISOString(), gender: 'female' }, now), 'была в сети вчера в 9:30');
  assert.equal(presence({ lastSeenAt: new Date(2026, 8, 1, 9, 30).toISOString() }, now), 'был в сети 1 сентября в 9:30');
  assert.equal(presence({ lastSeenAt: null }, now), '');
  assert.equal(onlineCount(1), '1 человек онлайн');
  assert.equal(onlineCount(3), '3 человека онлайн');
  assert.equal(onlineCount(5), '5 человек онлайн');
  assert.deepEqual(onlineReads.map(([, text]) => text), ['все', 'пользователи Freenet', 'те, на кого я подписан(а)', 'никто']);
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
  assert.equal(sharedBy(0), 'Никто не поделился');
  assert.equal(sharedBy(1), 'Поделился 1 человек');
  assert.equal(sharedBy(3), 'Поделились 3 человека');
  assert.equal(sharedBy(5), 'Поделились 5 человек');
  assert.equal(sharedBy(21), 'Поделился 21 человек');
});

test('isLongText is openvk\'s bigText', () => {
  assert.equal(isLongText('привет'), false);
  assert.equal(isLongText('я'.repeat(520)), false);
  assert.equal(isLongText('я'.repeat(521)), true);
  assert.equal(isLongText(Array(10).fill('строка').join('\n')), false);
  assert.equal(isLongText(Array(11).fill('строка').join('\n')), true);
});

test('forwarded messages nest under those that carried them on', () => {
  const names = (trees) => trees.map((t) => (t.carried.length ? [t.message.text, names(t.carried)] : t.message.text));

  assert.deepEqual(names(forwardTree([])), []);
  assert.deepEqual(names(forwardTree([{ text: 'a' }, { text: 'b' }])), ['a', 'b']);
  assert.deepEqual(names(forwardTree([
    { text: 'a', depth: 0 }, { text: 'b', depth: 1 }, { text: 'c', depth: 2 }, { text: 'd', depth: 1 }, { text: 'e', depth: 0 },
  ])), [['a', [['b', ['c']], 'd']], 'e']);
  // A level no message leads to is the deepest there is.
  assert.deepEqual(names(forwardTree([{ text: 'a', depth: 2 }, { text: 'b', depth: 3 }])), [['a', ['b']]]);
});

test('a preview of a message reads whom it names by name', () => {
  assert.equal(messagePreview({ text: '[ann|Анна], привет' }), 'Анна, привет');
  assert.equal(messagePreview({ text: 'спроси @ann' }), 'спроси @ann');
  assert.equal(chatPreview({ action: '', text: '[ann|Анна], иду', author: { name: 'Пётр' }, out: false }), 'Пётр: Анна, иду');
  assert.deepEqual(foundText({ text: '[ann|Анна], привет' }, 'прив'), { text: 'Анна, привет', forwarded: false });
  assert.deepEqual(foundText({ text: '', forwarded: [{ text: 'да, [ann|Анна]' }], forwardedCount: 1 }, 'анна'), { text: 'да, Анна', forwarded: true });
});

test('a message without words is named by what it shows', () => {
  assert.equal(messagePreview({ text: 'привет', photos: [{}] }), 'привет');
  assert.equal(messagePreview({ text: '', photos: [{}] }), 'Фотография');
  assert.equal(messagePreview({ text: '', photos: [], post: 'p1' }), 'Запись');
  assert.equal(messagePreview({ text: '', photos: [], albumPhoto: 'ph1' }), 'Фотография');
  assert.equal(messagePreview({ text: '', photos: [], post: '', forwardedCount: 1 }), 'Пересланное сообщение');
  assert.equal(messagePreview({ text: '', forwardedCount: 3 }), '3 пересланных сообщения');
  assert.equal(messagePreview({ text: '', deleted: true }), 'Сообщение удалено');
  assert.equal(messagePreview({ text: '' }), '');
  assert.equal(forwardedCount(5), '5 пересланных сообщений');
  assert.equal(forwardedCount(21), '21 пересланное сообщение');
  assert.equal(errorText({ message: 'forwarded message not found' }), 'Сообщение не удалось переслать: его уже нет.');
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
  assert.equal(pastEventsCount(0), 'Ни одной прошедшей встречи');
  assert.equal(pastEventsCount(1), '1 прошедшая встреча');
  assert.equal(pastEventsCount(3), '3 прошедшие встречи');
  assert.equal(pastEventsCount(5), '5 прошедших встреч');
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

test('the site counts itself as openvk does', () => {
  assert.deepEqual(aboutStatistics({ users: 1, online: 2, clubs: 5, posts: 21 }), [
    [1, 'пользователь'],
    [2, 'пользователя в сети'],
    [5, 'групп'],
    [21, 'запись на стенах'],
  ]);
});

test('the choices of the interface are those the API knows', () => {
  assert.deepEqual(avatarStyles.map(([v]) => v), ['square', 'round']);
  assert.deepEqual(mainPages.map(([v]) => v), ['page', 'feed']);
  assert.deepEqual(menuItems.map(([v]) => v), ['photos', 'messages', 'notes', 'groups', 'events', 'feed', 'notifications']);
  assert.equal(errorText({ message: 'invalid menu item' }), 'Неизвестный пункт меню.');
});

test('the rest of "Интерфейс" names what the server knows', () => {
  assert.deepEqual(themes.map(([v]) => v), ['default', 'blue', 'dark']);
  assert.deepEqual(ratings.map(([v]) => v), ['show', 'hide']);
  assert.deepEqual(nsfwContents.map(([v]) => v), ['hide', 'blur', 'show']);
  assert.deepEqual(postViews.map(([v]) => v), ['old', 'microblog']);

  // The browser's clock first, then every clock of openvk, west to east.
  const offsets = timezones.map(([v]) => v);
  assert.equal(offsets[0], '');
  assert.equal(offsets.length, 39);
  for (const offset of offsets.slice(1)) {
    const minutes = Number(offset);
    assert.ok(minutes >= -720 && minutes <= 840 && minutes % 15 === 0, offset);
  }
  assert.deepEqual(offsets.slice(1), [...offsets.slice(1)].sort((a, b) => a - b));
  assert.equal(label(timezones, '180'), '(GMT+3): Москва, Стамбул, Минск');
});

test('dates follow the clock of the settings', () => {
  const at = '2026-10-04T22:30:00Z';
  const now = new Date('2026-10-05T01:00:00Z');

  try {
    setTimezone(0);
    assert.equal(formatPostDate(at, now), 'вчера в 22:30');
    assert.equal(formatDateTime(at), '4 октября 2026 в 22:30');
    assert.equal(inputDate(at), '2026-10-04');
    assert.equal(inputTime(at), '22:30');
    assert.equal(localTime('2026-10-04', '22:30'), '2026-10-04T22:30:00.000Z');

    // Moscow is three hours ahead: the same moment is already tomorrow.
    setTimezone(180);
    assert.equal(formatPostDate(at, now), 'сегодня в 1:30');
    assert.equal(formatDateTime(at), '5 октября 2026 в 1:30');
    assert.equal(inputDate(at), '2026-10-05');
    assert.equal(inputTime(at), '01:30');
    assert.equal(localTime('2026-10-05', '01:30'), '2026-10-04T22:30:00.000Z');

    setTimezone(-210);
    assert.equal(formatPostDate(at, now), 'сегодня в 19:00');
    assert.equal(formatPostDate('2025-12-31T23:00:00Z', now), '31 декабря 2025 в 19:30');
  } finally {
    // The browser's own clock again, as the other tests expect.
    setTimezone(null);
  }

  assert.equal(formatPostDate(new Date(2026, 9, 4, 9, 5).toISOString(), new Date(2026, 9, 4, 12, 0)), 'сегодня в 9:05');
  assert.equal(localTime('2026-10-04', '09:05'), new Date(2026, 9, 4, 9, 5).toISOString());
  assert.equal(localTime('', '09:05'), '');
});

test('mentionParts finds the people a text names', () => {
  assert.deepEqual(mentionParts('привет'), ['привет']);
  assert.deepEqual(mentionParts(''), []);
  assert.deepEqual(mentionParts('[ann|Анна], привет'), [{ nickname: 'ann', text: 'Анна' }, ', привет']);
  assert.deepEqual(mentionParts('@ann привет, @bob_1!'),
    [{ nickname: 'ann', text: '@ann' }, ' привет, ', { nickname: 'bob_1', text: '@bob_1' }, '!']);
  assert.deepEqual(mentionParts('строка\n@ann'), ['строка\n', { nickname: 'ann', text: '@ann' }]);
});

test('mentionParts leaves what is not a mention', () => {
  assert.deepEqual(mentionParts('почта ann@example.com'), ['почта ann@example.com']);
  assert.deepEqual(mentionParts('@1ann и @'), ['@1ann и @']);
  assert.deepEqual(mentionParts('[ann|]'), ['[ann|]']);
  assert.deepEqual(mentionParts('[ann_|Анна]'), ['[ann_|Анна]']);
  assert.deepEqual(mentionParts('@' + 'a'.repeat(33)), ['@' + 'a'.repeat(33)]);
  // A nickname does not end with "_".
  assert.deepEqual(mentionParts('@ann_'), [{ nickname: 'ann', text: '@ann' }, '_']);
});

test('plainMentions reads a mention as the name', () => {
  assert.equal(plainMentions('[ann|Анна], привет @bob'), 'Анна, привет @bob');
});

test('replyMention starts an answer with a link to the person', () => {
  assert.equal(replyMention({ nickname: 'ann', name: 'Анна' }), '[ann|Анна], ');
  assert.equal(replyMention({ club: true, name: 'Кошки' }), 'Кошки, ');
});

test('notificationVerb says who mentioned', () => {
  assert.equal(notificationVerb('mention', { gender: 'female' }), 'упомянула');
  assert.equal(notificationVerb('mention', { club: true }), 'упомянуло');
});

test('foundParts marks every place a text says the query, whatever the case', () => {
  assert.deepEqual(foundParts('Привет, привет!', 'ПРИВЕТ'), [
    { text: 'Привет', found: true },
    { text: ', ', found: false },
    { text: 'привет', found: true },
    { text: '!', found: false },
  ]);
  assert.deepEqual(foundParts('привет', 'пока'), [{ text: 'привет', found: false }]);
  assert.deepEqual(foundParts('привет', '  '), [{ text: 'привет', found: false }]);
  assert.deepEqual(foundParts('', 'а'), [{ text: '', found: false }]);
});

test('foundSnippet starts a little before the words found', () => {
  assert.equal(foundSnippet('раз два три', 'два', 8), 'раз два…');
  assert.equal(foundSnippet('раз два три четыре пять шесть', 'пять', 8), '…е пять ш…');
  assert.equal(foundSnippet('раз два три', 'семь', 7), 'раз два…');
  assert.equal(foundSnippet('👋👋👋👋👋👋да нет', 'да', 4).includes('да'), true);
});

test('archiveYears lays out the years of the archive', () => {
  const years = [2026, 2025, 2024, 2023, 2022, 2021, 2020, 2019];

  assert.deepEqual(archiveYears(years), { shown: [2026, 2025, 2024, 2023, 2022, 2021], more: [2020, 2019], action: 2026 });
  assert.deepEqual(archiveYears(years, 2019).action, 2019);
  assert.deepEqual(archiveYears([2025, 2024], 2024), { shown: [2025, 2024], more: [], action: 2024 });
  // A year the wall has no posts of is shown all the same, in its place.
  assert.deepEqual(archiveYears([2026, 2024], 2025), { shown: [2026, 2025, 2024], more: [], action: 2025 });
  // An empty wall has nothing to choose.
  assert.deepEqual(archiveYears([]), { shown: [], more: [], action: 0 });
});

test('foundText is the text of a found message that says the query', () => {
  const carried = [{ text: 'горы' }, { text: 'Еду на МОРЕ' }, { text: 'море рядом' }];

  assert.deepEqual(foundText({ text: 'На море', forwarded: carried }, 'море'), { text: 'На море', forwarded: false });
  assert.deepEqual(foundText({ text: 'смотри', forwarded: carried, forwardedCount: 3 }, ' море '), { text: 'Еду на МОРЕ', forwarded: true });
  assert.deepEqual(foundText({ text: '', forwarded: carried, forwardedCount: 3 }, 'горы'), { text: 'горы', forwarded: true });
  // The text was cut before what was found, or its author's page is gone.
  assert.deepEqual(foundText({ text: 'смотри', forwarded: [{ text: 'горы' }], forwardedCount: 2 }, 'море'), { text: 'смотри', forwarded: false });
  assert.deepEqual(foundText({ text: '', forwardedCount: 2 }, 'море'), { text: '2 пересланных сообщения', forwarded: false });
});

test('the list of sessions says how many devices', () => {
  assert.equal(sessionsCount(1), 'Вы вошли на сайт с 1 устройства.');
  assert.equal(sessionsCount(2), 'Вы вошли на сайт с 2 устройств.');
  assert.equal(sessionsCount(21), 'Вы вошли на сайт с 21 устройства.');
});

test('a session is named by its browser and system', () => {
  const names = [
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36', 'Chrome, Windows'],
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0', 'Edge, Windows'],
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 YaBrowser/25.8.0.0 Safari/537.36', 'Яндекс Браузер, Windows'],
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 OPR/124.0.0.0', 'Opera, Windows'],
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15', 'Safari, macOS'],
    ['Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0', 'Firefox, Linux'],
    ['Mozilla/5.0 (Linux; Android 16; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36', 'Chrome, Android'],
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1', 'Safari, iPhone'],
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/140.0.0.0 Mobile/15E148 Safari/604.1', 'Chrome, iPhone'],
    ['Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36', 'Chrome, ChromeOS'],
    ['curl/8.7.1', 'curl/8.7.1'],
    ['Python-urllib/3.13 (Linux)', 'Linux'],
    ['x'.repeat(80), 'x'.repeat(60) + '…'],
    ['', 'Неизвестное устройство'],
    [undefined, 'Неизвестное устройство'],
  ];

  for (const [agent, name] of names) assert.equal(deviceName(agent), name, agent);
});

test('pollVoters tells how many voted, as openvk does', () => {
  assert.equal(pollVoters(0), 'Будьте первым, кто проголосует!');
  assert.equal(pollVoters(1), 'В опросе проголосовал 1 человек.');
  assert.equal(pollVoters(3), 'В опросе проголосовало 3 человека.');
  assert.equal(pollVoters(11), 'В опросе проголосовало 11 человек.');
  assert.equal(pollVoters(21), 'В опросе проголосовал 21 человек.');
});

test('pollMeta tells how a poll is held', () => {
  const now = new Date('2026-10-11T12:00:00Z');

  assert.equal(pollMeta({ anonymous: false, multiple: false, locked: false, endsAt: null }, now), 'Публичное голосование');
  assert.equal(pollMeta({ anonymous: true, multiple: true, locked: true, endsAt: null }, now),
    'Анонимное голосование • много вариантов • нельзя переголосовать');

  setTimezone(0);
  try {
    assert.equal(pollMeta({ anonymous: false, multiple: false, locked: true, endsAt: '2026-10-14T15:30:00Z' }, now),
      'Публичное голосование • нельзя переголосовать • до 14 октября в 15:30');
  } finally {
    setTimezone(null);
  }
});

test('pollPercent rounds the share of the voters down', () => {
  assert.equal(pollPercent(0, 0), 0);
  assert.equal(pollPercent(1, 3), 33);
  assert.equal(pollPercent(2, 3), 66);
  assert.equal(pollPercent(3, 3), 100);
});

test('pollAsksVote shows the options until the reader votes or the poll ends', () => {
  assert.equal(pollAsksVote({ ended: false, voted: [] }), true);
  assert.equal(pollAsksVote({ ended: false, voted: [1] }), false);
  assert.equal(pollAsksVote({ ended: true, voted: [] }), false);
});

test('daysCount declines the days of a poll', () => {
  assert.equal(daysCount(1), '1 день');
  assert.equal(daysCount(3), '3 дня');
  assert.equal(daysCount(11), '11 дней');
  assert.equal(daysCount(31), '31 день');
});

test('errorText knows what a poll refuses', () => {
  assert.equal(errorText(new Error('you have voted in the poll already')), 'Вы уже проголосовали в этом опросе.');
  assert.equal(errorText(new Error('the poll has ended')), 'Голосование окончено.');
});

test('feedSettings takes what the browser kept and the defaults for the rest', () => {
  assert.deepEqual(feedSettings(null), { posts: 20, alien: true });
  assert.deepEqual(feedSettings('{"posts":5,"alien":false}'), { posts: 5, alien: false });
  assert.deepEqual(feedSettings('{"posts":7,"alien":"no"}'), { posts: 20, alien: true });
  assert.deepEqual(feedSettings('not json'), { posts: 20, alien: true });
  assert.deepEqual(feedSettings('[]'), { posts: 20, alien: true });
  assert.ok(feedPageSizes.every((n) => n >= 1 && n <= 50));
});

test('ignoreLabel names the link of a page as openvk does', () => {
  assert.equal(ignoreLabel('user', false), 'Игнорировать пользователя');
  assert.equal(ignoreLabel('user', true), 'Не игнорировать пользователя');
  assert.equal(ignoreLabel('club', false), 'Игнорировать группу');
  assert.equal(ignoreLabel('club', true), 'Не игнорировать группу');
  assert.equal(ignoreLabel('event', true), 'Не игнорировать встречу');
});

test('errorText knows what ignoring refuses', () => {
  assert.equal(errorText(new Error('too many ignored sources')), 'Превышен лимит игнорируемых источников.');
  assert.equal(errorText(new Error('you cannot ignore yourself')), 'Нельзя игнорировать самого себя.');
});

test('chats are named, counted and told of', () => {
  const ann = { name: 'Анна', surname: 'Иванова', gender: 'female' };
  const petr = { name: 'Пётр', surname: 'Петров', gender: 'male' };

  assert.equal(chatTitle({ title: 'Поход' }), 'Поход');
  assert.equal(chatTitle({ title: '' }), 'Беседа');

  assert.equal(defaultChatTitle([ann, petr]), 'Анна, Пётр');
  assert.equal(defaultChatTitle([]), '');
  // As many names as 64 characters hold, never half a name.
  const many = Array.from({ length: 20 }, () => ({ name: 'Александр' }));
  const title = defaultChatTitle(many);
  assert.ok([...title].length <= 64 && title.endsWith('Александр') && title.split(', ').length === 6);

  assert.equal(chatMembers(1), '1 участник');
  assert.equal(chatMembers(3), '3 участника');
  assert.equal(chatMembers(50), '50 участников');

  assert.equal(accusativeName(petr), 'Петра Петрова');
  assert.equal(accusativeName(ann), 'Анну Иванову');
  assert.equal(accusativeName({ name: 'Мария', surname: 'Белая', gender: 'female' }), 'Марию Белую');
  assert.equal(accusativeName({ name: 'Ann', surname: 'Lee', gender: 'female' }), 'Ann Lee');

  assert.equal(chatAction({ action: 'create', text: 'Поход', author: ann }), 'создала беседу «Поход»');
  assert.equal(chatAction({ action: 'create', text: '', author: petr }), 'создал беседу');
  assert.equal(chatAction({ action: 'title', text: 'Горы', author: petr }), 'изменил название беседы на «Горы»');
  assert.equal(chatAction({ action: 'title', text: '', author: ann }), 'убрала название беседы');
  assert.equal(chatAction({ action: 'invite', author: ann, target: petr }), 'пригласила Петра Петрова');
  assert.deepEqual(chatActionParts({ action: 'kick', author: petr, target: ann }), ['исключил ', 'Анну Иванову', ' из беседы']);
  assert.equal(chatAction({ action: 'leave', author: { gender: 'female' } }), 'покинула беседу');
  assert.equal(chatAction({ action: 'return', author: ann }), 'вернулась в беседу');
  assert.equal(chatAction({ action: '', author: ann }), '');

  assert.equal(chatPreview(null), '');
  assert.equal(chatPreview({ action: 'leave', author: petr }), 'Пётр покинул беседу');
  assert.equal(chatPreview({ action: '', text: 'иду', author: petr, out: false }), 'Пётр: иду');
  assert.equal(chatPreview({ action: '', text: 'иду', author: ann, out: true }), 'иду');
  assert.equal(chatPreview({ action: '', text: '', photos: [{}], author: petr }), 'Пётр: Фотография');
  assert.equal(chatPreview({ action: '', deleted: true, text: '', author: petr }), 'Пётр: Сообщение удалено');

  assert.equal(chatStateText('kicked'), 'Вас исключили из беседы.');
  assert.equal(chatStateText('left'), 'Вы покинули беседу.');

  assert.deepEqual(chatsInvites.map(([value]) => value), ['everyone', 'following', 'nobody']);
  assert.equal(errorText({ message: 'you cannot add this user to a chat' }), 'Пользователь запретил добавлять себя в беседы.');
  assert.equal(errorText({ message: 'a chat needs at least two other people' }), 'В беседе должно быть как минимум два других пользователя.');
});
