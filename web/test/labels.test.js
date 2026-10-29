// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';

import { declinedName, partnerName, worldviews, politicalViews, photoChanges, forwardTree, forwardedCount, messagePreview, mentionParts, plainMentions, replyMention, isLongText, selectedCount, partnerPrefix, takesPartner, albumsCount, myAlbumsCount, photosCount, topicsCount, topicMessages, topicMessagesCount, resultsCount, membersCount, myClubsCount, clubsCount, adminsCount, suggestedByEveryone, suggestedByYou, suggestedInClub, audiences, pageReads, onlineReads, presence, wallCount, completenessHints, likedBy, sharedBy, adminsDisplays, label, relationships, genders, formatBirthday, errorText, formatPostDate, postsCount, wrote, followersCount, followingCount, notificationVerb, unreadCountText, snippet, foundParts, foundSnippet, foundText, genitiveName, wallWrites, blacklistCount, sessionsCount, deviceName, limitedAccess, messagesWrites, formatDateTime, deactivationReasons, localTime, inputDate, inputTime, aboutStatistics, adminOverview, adminEconomy, avatarStyles, mainPages, menuItems, themes, ratings, nsfwContents, postViews, timezones, setTimezone, archiveYears, pollVoters, pollMeta, pollPercent, pollAsksVote, daysCount, favesCount, feedSettings, feedPageSizes, ignoreLabel, chatTitle, defaultChatTitle, chatMembers, chatAction, chatActionParts, chatPreview, chatStateText, accusativeName, chatsInvites, conversationValue, conversationOf, conversationPath, profileTypes, clubTypes, clubTypeHints, requestsCount, requestsLink, closedFollows, giftsCount, giftCollections, giftNames, giftImage, votesCount, giftPrice, unusedVotes, voucherParts, voucherCode, voucherUsages, voucherStatus, voucherGave, stickerPacks, stickerImage, stickerPackName, stickerPackAction, postponedCount, publishesAt, nextHour, typingText, DOCUMENT_FORMATS, documentFormat, documentsCount, haveDocuments, fileSize, inviteNickname, inviteURL, linkParts, shownLink, textParts, banUntil, banEnd, rubles, voteRate, foreignAgentText, adRate, peopleCount, newYearGreeting, casinoChances, casinoChance, casinoReturn, casinoPayout, casinoPrize, spinsCount, casinoPlan, wagersCount, betMultiplier, aimReturn, betOddsField, typedOdds, betChance, betRoll, betPrize, betAmount, aimZone, betEventShare, betEventOdds, betEventState, autoplayNext, votesFontSize, groupedVotes, votesAmount, signedVotes, voteOperation, transferNickname, transferFee, stickerIncome, transferURL, rocketMultiplier, rocketTime, rocketShown, rocketsCount, rocketScale, primePeriods, primeSaving, primeTerms, titleIcons, titleIconPath, titleGroups, topValue } from '../static/labels.js';

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

test('a comment of someone without a page is refused', () => {
  assert.equal(errorText(new Error('fill in your profile before commenting')), 'Сначала заполните свою страницу.');
});

test('notification verbs agree with the gender of who did it', () => {
  assert.equal(notificationVerb('like', { gender: 'female' }), 'оценила');
  assert.equal(notificationVerb('comment', { gender: 'male' }), 'оставил');
  assert.equal(notificationVerb('follow', { gender: '' }), 'подписался(-ась)');
  assert.equal(notificationVerb('comment_reply', { gender: 'female' }), 'ответила');
  assert.equal(notificationVerb('comment_like', { gender: '' }), 'оценил(а)');
  assert.equal(notificationVerb('verified', { gender: 'female' }), 'подтвердила');
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

test('names that lose a vowel or are declined like adjectives', () => {
  const cases = [
    [{ name: 'Витёк', surname: 'Кравец', gender: 'male' }, 'Витька Кравца'],
    [{ name: 'Иван', surname: 'Стрелец', gender: 'male' }, 'Ивана Стрельца'],
    [{ name: 'Иван', surname: 'Коломиец', gender: 'male' }, 'Ивана Коломийца'],
    [{ name: 'Иван', surname: 'Кузнец', gender: 'male' }, 'Ивана Кузнеца'],
    [{ name: 'Иван', surname: 'Швец', gender: 'male' }, 'Ивана Швеца'],
    [{ name: 'Иван', surname: 'Заяц', gender: 'male' }, 'Ивана Зайца'],
    [{ name: 'Иван', surname: 'Волчок', gender: 'male' }, 'Ивана Волчка'],
    [{ name: 'Иван', surname: 'Королёк', gender: 'male' }, 'Ивана Королька'],
    [{ name: 'Александр', surname: 'Блок', gender: 'male' }, 'Александра Блока'],
    [{ name: 'Иван', surname: 'Игрок', gender: 'male' }, 'Ивана Игрока'],
    [{ name: 'Анна', surname: 'Кравец', gender: 'female' }, 'Анны Кравец'],
    [{ name: 'Виктор', surname: 'Цой', gender: 'male' }, 'Виктора Цоя'],
    [{ name: 'Иван', surname: 'Палий', gender: 'male' }, 'Ивана Палия'],
    [{ name: 'Иван', surname: 'Горячий', gender: 'male' }, 'Ивана Горячего'],
    [{ name: 'Анна', surname: 'Горячая', gender: 'female' }, 'Анны Горячей'],
    [{ name: 'Анна', surname: 'Сковорода', gender: 'female' }, 'Анны Сковороды'],
    [{ name: 'Андре', surname: 'Моруа', gender: 'male' }, 'Андре Моруа'],
    [{ name: 'Любовь', surname: 'Орлова' }, 'Любови Орловой'],
    [{ name: 'ПАВЕЛ', surname: 'ЗАЯЦ', gender: 'male' }, 'ПАВЛА ЗАЙЦА'],
  ];

  for (const [person, want] of cases) {
    assert.equal(genitiveName(person), want, JSON.stringify(person));
  }
});

test('names in the other cases', () => {
  const cases = [
    [{ name: 'Пётр', surname: 'Петров', gender: 'male' }, 'Петра Петрова', 'Петром Петровым', 'Петре Петрове'],
    [{ name: 'Анна', surname: 'Иванова', gender: 'female' }, 'Анну Иванову', 'Анной Ивановой', 'Анне Ивановой'],
    [{ name: 'Мария', surname: 'Белая', gender: 'female' }, 'Марию Белую', 'Марией Белой', 'Марии Белой'],
    [{ name: 'Лев', surname: 'Толстой', gender: 'male' }, 'Льва Толстого', 'Львом Толстым', 'Льве Толстом'],
    [{ name: 'Андрей', surname: 'Достоевский', gender: 'male' }, 'Андрея Достоевского', 'Андреем Достоевским', 'Андрее Достоевском'],
    [{ name: 'Дмитрий', surname: 'Гоголь', gender: 'male' }, 'Дмитрия Гоголя', 'Дмитрием Гоголем', 'Дмитрии Гоголе'],
    [{ name: 'Илья', surname: 'Кравец', gender: 'male' }, 'Илью Кравца', 'Ильёй Кравцом', 'Илье Кравце'],
    [{ name: 'Саша', surname: 'Шевченко', gender: 'male' }, 'Сашу Шевченко', 'Сашей Шевченко', 'Саше Шевченко'],
    [{ name: 'Ольга', surname: 'Шевчук', gender: 'female' }, 'Ольгу Шевчук', 'Ольгой Шевчук', 'Ольге Шевчук'],
    [{ name: 'Любовь', surname: 'Чайковская', gender: 'female' }, 'Любовь Чайковскую', 'Любовью Чайковской', 'Любови Чайковской'],
    [{ name: 'Павел', surname: 'Бабич', gender: 'male' }, 'Павла Бабича', 'Павлом Бабичем', 'Павле Бабиче'],
    [{ name: 'Ann', surname: 'Lee', gender: 'female' }, 'Ann Lee', 'Ann Lee', 'Ann Lee'],
  ];

  for (const [person, accusative, instrumental, prepositional] of cases) {
    assert.equal(declinedName(person, 'acc'), accusative, JSON.stringify(person));
    assert.equal(declinedName(person, 'ins'), instrumental, JSON.stringify(person));
    assert.equal(declinedName(person, 'pre'), prepositional, JSON.stringify(person));
  }

  assert.equal(declinedName({ name: 'Пётр', surname: 'Петров' }, 'nom'), 'Пётр Петров');
});

test('the partner of a relationship is in the case its prefix asks for', () => {
  const petr = { name: 'Пётр', surname: 'Петров', gender: 'male' };
  const ann = { name: 'Анна', surname: 'Иванова', gender: 'female' };

  assert.equal(partnerName('married', 'female', petr), 'Петром Петровым');
  assert.equal(partnerName('married', 'male', ann), 'Анне Ивановой');
  assert.equal(partnerName('married', '', ann), 'Анной Ивановой');
  assert.equal(partnerName('in_love', 'female', petr), 'Петра Петрова');
  assert.equal(partnerName('dating', 'male', ann), 'Анной Ивановой');
});

test('a wall post notification and its error read like openvk', () => {
  assert.equal(notificationVerb('wall_post', { gender: 'female' }), 'написала');
  assert.equal(notificationVerb('wall_post', { gender: '' }), 'написал(а)');
  assert.equal(errorText(new Error('you cannot post on this wall')), 'Хозяин страницы не разрешает вам писать у него на стене.');
  assert.deepEqual(wallWrites.map(([value]) => value), ['everyone', 'following', 'only_me']);
});

test('a repost is told as on openvk', () => {
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
  assert.equal(limitedAccess('neutral'), 'ограничил(а)');
  assert.equal(notificationVerb('like', { gender: 'neutral' }), 'оценил(а)');
  assert.equal(label(genders, 'neutral'), '');
  assert.equal(label(relationships('neutral'), 'married'), 'в браке');
});

test('what a select offers is written in lower case', () => {
  const lists = [genders, worldviews, politicalViews, wallWrites, messagesWrites, chatsInvites, profileTypes, clubTypes, audiences, pageReads, onlineReads, avatarStyles, themes, ratings, nsfwContents, postViews, mainPages];

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
  assert.deepEqual(onlineReads.map(([, text]) => text), ['все', 'пользователи Freenet', 'те, на кого я подписан(а)', 'никто']);
});

test('an album the site keeps refuses changes in words', () => {
  assert.equal(errorText(new Error('this album is kept by the site')), 'Этот альбом сайт ведёт сам: его нельзя изменить.');
});

test('a wall is counted as openvk counts it', () => {
  assert.equal(wallCount(0), 'нет записей');
  assert.equal(wallCount(1), '1 запись');
  assert.equal(wallCount(25), '25 записей');
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

test('a time goes through the inputs and back', () => {
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

test('the admin panel counts the site as openvk\'s "Сводка"', () => {
  assert.deepEqual(adminOverview({ users: 1, online: 0, verified: 1, clubs: 3, posts: 25 }), {
    users: 'зарегистрированный пользователь:',
    online: 'онлайн',
    verified: 'верифицированный',
    clubs: 'группы',
    posts: 'записей',
  });
  assert.equal(adminOverview({ users: 12, online: 0, clubs: 0, posts: 0 }).users, 'зарегистрированных пользователей, из них:');
  assert.equal(adminOverview({ users: 12, online: 0, verified: 3, clubs: 0, posts: 0 }).verified, 'верифицированных');
});

test('the choices of the interface are those the API knows', () => {
  assert.deepEqual(avatarStyles.map(([v]) => v), ['square', 'round']);
  assert.deepEqual(mainPages.map(([v]) => v), ['page', 'feed']);
  assert.deepEqual(menuItems.map(([v]) => v), ['messages', 'groups', 'feed', 'notifications']);
  assert.equal(errorText({ message: 'invalid menu item' }), 'Неизвестный пункт меню.');
});

test('the rest of "Интерфейс" names what the server knows', () => {
  assert.deepEqual(themes.map(([v]) => v), ['default', 'blue', 'dark', 'gold']);
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
  const by = (posts, alien, order = 'interesting') => ({ posts, alien, order });
  assert.deepEqual(feedSettings(null), by(10, true));
  assert.deepEqual(feedSettings('{"posts":5,"alien":true}'), by(5, true));
  assert.deepEqual(feedSettings('{"posts":5,"alien":false,"order":"time"}'), by(5, false, 'time'));
  assert.deepEqual(feedSettings('{"posts":7,"alien":"no","order":"random"}'), by(10, true));
  assert.deepEqual(feedSettings('not json'), by(10, true));
  assert.deepEqual(feedSettings('[]'), by(10, true));
  assert.ok(feedPageSizes.every((n) => n >= 1 && n <= 50));
});

test('ignoreLabel names the link of a page as openvk does', () => {
  assert.equal(ignoreLabel('user', false), 'Игнорировать пользователя');
  assert.equal(ignoreLabel('user', true), 'Не игнорировать пользователя');
  assert.equal(ignoreLabel('club', false), 'Игнорировать группу');
  assert.equal(ignoreLabel('club', true), 'Не игнорировать группу');
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
  assert.equal(chatAction({ action: 'pin', author: ann }), 'закрепила сообщение');
  assert.equal(chatAction({ action: 'unpin', author: petr }), 'открепил сообщение');
  assert.equal(chatAction({ action: 'photo', author: ann }), 'обновила фотографию беседы');
  assert.equal(chatAction({ action: 'photo_remove', author: petr }), 'удалил фотографию беседы');
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

test('photoChanges names what an edit adds and takes away', () => {
  assert.deepEqual(photoChanges([{ id: 'a' }, { id: 'b' }], ['b', 'c']), { add: ['c'], remove: ['a'] });
  assert.deepEqual(photoChanges(undefined, ['a']), { add: ['a'], remove: [] });
  assert.deepEqual(photoChanges([{ id: 'a' }], ['a']), { add: [], remove: [] });
});

test('a conversation is named by a value that tells a dialog from a chat', () => {
  assert.equal(conversationValue({ peer: { nickname: 'ann' } }), 'user:ann');
  assert.equal(conversationValue({ chat: { id: '0198' } }), 'chat:0198');
  assert.deepEqual(conversationOf('user:ann'), { chat: false, ref: 'ann' });
  assert.deepEqual(conversationOf('chat:0198'), { chat: true, ref: '0198' });
  assert.equal(conversationPath('user:ann'), '/im/ann');
  assert.equal(conversationPath('chat:0198'), '/im/chat/0198');
});

test('chatPreview says what a message of a chat shows without words', () => {
  const author = { name: 'Пётр' };

  assert.equal(chatPreview({ action: '', text: '', post: 'p1', author, out: false }), 'Пётр: Запись');
  assert.equal(chatPreview({ action: '', text: '', albumPhoto: 'f1', author, out: true }), 'Фотография');
  assert.equal(chatPreview({ action: '', text: '', forwardedCount: 2, author, out: false }), 'Пётр: 2 пересланных сообщения');
});

test('closed profiles and groups speak as openvk does', () => {
  assert.deepEqual(profileTypes.map(([value]) => value), ['open', 'closed']);
  assert.deepEqual(clubTypes.map(([value]) => value), ['open', 'closed']);
  assert.equal(clubTypeHints.closed, 'Это закрытая группа. Для вступления необходимо подавать заявку.');
  assert.equal(requestsCount(0), 'Не найдено ни одной заявки...');
  assert.equal(requestsCount(1), 'Найдена 1 заявка');
  assert.equal(requestsCount(3), 'Найдено 3 заявки');
  assert.equal(requestsCount(5), 'Найдено 5 заявок');
  assert.equal(requestsLink(2), '2 заявки на вступление');
  assert.equal(closedFollows('female'), 'подписана сама');
  assert.equal(errorText({ message: 'this club is closed' }), clubTypeHints.closed);
});

test('gifts are counted, named and pictured as the catalogue of the user service has them', () => {
  assert.equal(giftsCount(0), 'Нет подарков');
  assert.equal(giftsCount(1), '1 подарок');
  assert.equal(giftsCount(3), '3 подарка');
  assert.equal(giftsCount(15), '15 подарков');
  assert.equal(notificationVerb('gift', { gender: 'female' }), 'отправила');
  assert.equal(errorText({ message: "this user's gifts are hidden" }), 'Пользователь скрыл свои подарки.');
  assert.equal(errorText({ message: 'you cannot send a gift to this user' }), 'Вы не можете отправить подарок этому пользователю.');

  // The catalogue is the domain's: every collection and gift it has is
  // named here and has its picture among the files of the site.
  const source = readFileSync(new URL('../../user/internal/user/gift.go', import.meta.url), 'utf8');
  const catalog = /var giftCatalog = [^{]*\{([\s\S]*?)\n\}/.exec(source)[1];
  const collections = [...catalog.matchAll(/ID:\s*"(\w+)"/g)].map((m) => m[1]);
  const gifts = [...catalog.matchAll(/Gifts:[^{]*\{([^}]*)\}/g)].flatMap((m) => [...m[1].matchAll(/"(\w+)"/g)].map((g) => g[1]));

  assert.deepEqual(Object.keys(giftCollections), collections);
  assert.equal(gifts.length, 11);
  assert.deepEqual(Object.keys(giftNames), gifts);
  for (const gift of gifts) {
    // The gifts are pictures rendered by scripts/giftgen.
    const file = `${gift}.png`;
    assert.equal(giftImage(gift), `/static/gifts/${file}`);
    assert.ok(existsSync(new URL(`../static/gifts/${file}`, import.meta.url)), gift);
  }
  assert.equal(giftImage('rose'), '/static/gifts/unknown.svg');
});

test('stickers are pictured by the photos people made them of', () => {
  // The site's own packs are gone: the catalogue of the user service is empty.
  const source = readFileSync(new URL('../../user/internal/user/sticker.go', import.meta.url), 'utf8');
  assert.match(source, /var stickerCatalog \[\]StickerPack\n/);
  assert.equal(stickerImage('cat_hi'), null);
  assert.equal(stickerImage('unknown'), null);
  // A sticker of a pack a person made is a photo the gateway serves.
  assert.equal(stickerImage('0198c0de000070008000000000000001'), '/media/stickers/0198c0de000070008000000000000001');
  assert.equal(stickerImage('0198C0DE000070008000000000000001'), null);
  assert.equal(stickerPackName({ id: 'cats' }), 'Котики');
  assert.equal(stickerPackName({ id: '0198c0de000070008000000000000001', name: 'Мой кот' }), 'Мой кот');
  assert.equal(stickerPackName({ id: 'newer' }), 'Стикеры');
  assert.equal(errorText({ message: 'the sticker pack is taken by others' }), 'Набор уже установили другие: удалить его или его стикеры нельзя.');

  // The button of openvk's shop: a pack once bought comes back free.
  assert.equal(stickerPackAction({ price: 0 }), 'Добавить');
  assert.equal(stickerPackAction({ price: 2 }), 'Купить за 2 голоса');
  assert.equal(stickerPackAction({ price: 4, bought: true }), 'Добавить');
  assert.equal(stickerPackAction({ price: 4, bought: true, added: true }), 'Приобретён');

  assert.equal(messagePreview({ sticker: 'cat_hi', text: '' }), 'Стикер');
  assert.equal(errorText({ message: 'you do not have this sticker' }), 'Набор с этим стикером не добавлен.');
});

test('votes are counted and vouchers shown as openvk shows them', () => {
  assert.equal(votesCount(0), '0 голосов');
  assert.equal(votesCount(1), '1 голос');
  assert.equal(votesCount(3), '3 голоса');
  assert.equal(votesCount(11), '11 голосов');
  assert.equal(votesCount(3000000), '3\u00a0000\u00a0000 голосов');
  assert.equal(giftPrice(0), 'Бесплатно');
  assert.equal(giftPrice(3), '3 голоса');
  assert.deepEqual(unusedVotes(1), ['У Вас ', '1', ' неиспользованный голос.']);
  assert.deepEqual(unusedVotes(5), ['У Вас ', '5', ' неиспользованных голосов.']);

  assert.deepEqual(voucherParts('ABCDEFGH12345678JKLMNPQR'), ['ABCDEF', 'GH1234', '5678JK', 'LMNPQR']);
  assert.equal(voucherCode('ABCDEFGH12345678JKLMNPQR'), 'ABCDEF-GH1234-5678JK-LMNPQR');
  assert.equal(voucherCode(''), '');
  assert.equal(voucherUsages(-1), '∞');
  assert.equal(voucherUsages(0), '0');
  assert.equal(voucherUsages(12), '12');
  assert.equal(voucherStatus({ usagesLeft: 0 }), 'закончился');
  assert.equal(voucherStatus({ usagesLeft: -1 }), 'активен');
  assert.equal(voucherStatus({ usagesLeft: 2 }), 'активен');
  assert.equal(voucherGave({ votes: 10, rating: 5 }), '10 голосов и 5% рейтинга');
  assert.equal(voucherGave({ votes: 1, rating: 0 }), '1 голос');
  assert.equal(voucherGave({ votes: 0, rating: 30 }), '30% рейтинга');

  assert.equal(notificationVerb('votes', { gender: 'female' }), 'передала');
  assert.equal(errorText({ message: 'not enough votes' }), 'У вас недостаточно голосов.');
  assert.match(errorText({ message: 'voucher is not valid' }), /^Ваучер недействительный/);

  // What the user service refuses about votes and vouchers is said in
  // Russian: every message of its domain has a text.
  for (const file of ['wallet.go', 'voucher.go']) {
    const source = readFileSync(new URL('../../user/internal/user/' + file, import.meta.url), 'utf8');
    for (const [, message] of source.matchAll(/errors\.New\("([^"]+)"\)/g)) {
      if (message === 'only administrators can do this') continue; // the gateway says "permission denied"
      assert.notEqual(errorText({ message }), errorText({ message: 'no such message' }), message);
    }
  }
});

test('postponed posts', () => {
  assert.equal(postponedCount(0), 'Нет отложенных записей');
  assert.equal(postponedCount(1), '1 отложенная запись');
  assert.equal(postponedCount(3), '3 отложенные записи');
  assert.equal(postponedCount(11), '11 отложенных записей');
  setTimezone(180);
  assert.equal(publishesAt('2026-10-16T07:00:00Z'), 'Будет опубликована 16 октября 2026 в 10:00');
  setTimezone(null);
  assert.equal(nextHour(new Date('2026-10-15T12:34:56Z')), '2026-10-15T13:00:00.000Z');
  assert.equal(nextHour(new Date('2026-10-15T23:00:00Z')), '2026-10-16T00:00:00.000Z');
  assert.equal(errorText({ message: 'too many postponed posts' }), 'Отложено слишком много записей: не больше 100 на стену.');
});

test('who is typing', () => {
  const ann = { id: '1', name: 'Анна', nickname: 'ann' };
  const peter = { id: '2', name: 'Пётр', nickname: 'peter' };

  assert.equal(typingText([]), '');
  assert.equal(typingText([ann]), 'Анна печатает...');
  assert.equal(typingText([ann, peter]), 'Анна и Пётр печатают...');
  assert.equal(typingText([ann, peter, { id: '3', name: '', nickname: 'cat' }]), 'Несколько человек печатают...');
  assert.equal(typingText([{ id: '3', name: '', nickname: 'cat' }]), 'cat печатает...');
});

test('documents: formats as the post service takes them, counts and sizes', () => {
  const source = readFileSync(new URL('../../post/internal/post/document.go', import.meta.url), 'utf8');
  const listed = source.match(/var documentFormats = \[\]string\{([^}]*)\}/)[1].match(/"[^"]+"/g).map((s) => s.slice(1, -1));
  assert.deepEqual(DOCUMENT_FORMATS, listed);

  assert.equal(documentFormat('Отчёт.v2.PDF'), 'pdf');
  assert.equal(documentFormat('README'), '');

  assert.equal(documentsCount(1), '1 документ');
  assert.equal(documentsCount(3), '3 документа');
  assert.equal(haveDocuments(11), 'У Вас 11 документов');

  assert.equal(fileSize(512), '512 Б');
  assert.equal(fileSize(1536), '1,5 КБ');
  assert.equal(fileSize(2048), '2 КБ');
  assert.equal(fileSize(10 << 20), '10 МБ');

  assert.equal(messagePreview({ documentCount: 2 }), 'Документ');
  assert.equal(messagePreview({ text: 'вот', documentCount: 2 }), 'вот');
});

test('inviteNickname takes a nickname of the link and nothing else', () => {
  assert.equal(inviteNickname('?invite=ann_lee'), 'ann_lee');
  assert.equal(inviteNickname('?q=1&invite=Ann'), 'Ann');
  assert.equal(inviteNickname(''), '');
  assert.equal(inviteNickname('?invite=ann_'), '');
  assert.equal(inviteNickname('?invite=1ann'), '');
  assert.equal(inviteNickname('?invite=%3Cb%3E'), '');
  assert.equal(inviteNickname('?invite=' + 'a'.repeat(33)), '');
});

test('inviteURL invites to the main page', () => {
  assert.equal(inviteURL('https://freenet19.fun', 'ann'), 'https://freenet19.fun/?invite=ann');
  assert.equal(notificationVerb('invite', { gender: 'female' }), 'присоединилась');
});

test('linkParts finds web addresses as the gateway does', () => {
  assert.deepEqual(linkParts('нет ссылок'), ['нет ссылок']);
  assert.deepEqual(linkParts('Смотрите: https://example.com/a?b=1#c.'), [
    'Смотрите: ', { url: 'https://example.com/a?b=1#c', text: 'example.com/a?b=1#c' }, '.',
  ]);
  assert.deepEqual(linkParts('(https://ru.wikipedia.org/wiki/Кот_(значения))'), [
    '(', { url: 'https://ru.wikipedia.org/wiki/Кот_(значения)', text: 'ru.wikipedia.org/wiki/Кот_(значения)' }, ')',
  ]);
  assert.deepEqual(linkParts('«http://www.example.com»'), ['«', { url: 'http://www.example.com', text: 'example.com' }, '»']);
  assert.deepEqual(linkParts('ftp://example.com xhttps://example.com https://'), ['ftp://example.com xhttps://example.com https://']);
});

test('shownLink cuts a long address', () => {
  const shown = shownLink('https://example.com/' + 'a'.repeat(100));
  assert.equal([...shown].length, 60);
  assert.ok(shown.endsWith('…'));
});

test('textParts leaves "#" and "@" of an address to it', () => {
  assert.deepEqual(textParts('#кошки https://medium.com/@ann#top @bob'), [
    { hashtag: 'кошки', text: '#кошки' }, ' ',
    { url: 'https://medium.com/@ann#top', text: 'medium.com/@ann#top' }, ' ',
    { nickname: 'bob', text: '@bob' },
  ]);
  assert.deepEqual(textParts('[ann|https://example.com]'), [{ nickname: 'ann', text: 'https://example.com' }]);
});

test('giftCosts: a gift priced apart from its collection', async () => {
  const { giftCosts } = await import('../static/labels.js');
  assert.deepEqual(giftCosts({ gifts: ['freenet_coin', 'freenet_ring'], price: 1, prices: [4, 0] }), [{ gift: 'freenet_coin', price: 4 }, { gift: 'freenet_ring', price: 0 }]);
  // A server that does not say what each costs prices them as the collection.
  assert.deepEqual(giftCosts({ gifts: ['freenet_cup'], price: 2 }), [{ gift: 'freenet_cup', price: 2 }]);
  assert.deepEqual(giftCosts({ gifts: ['freenet_cup'], price: 2, prices: [] }), [{ gift: 'freenet_cup', price: 2 }]);
});

test('banUntil ends a block after a term, at a date or never', () => {
  const now = new Date('2026-10-19T12:00:00Z');

  assert.equal(banUntil('day', now), '2026-10-20T12:00:00.000Z');
  assert.equal(banUntil('week', now), '2026-10-26T12:00:00.000Z');
  assert.equal(banUntil('month', now), '2026-11-19T12:00:00.000Z');
  assert.equal(banUntil('year', now), '2027-10-19T12:00:00.000Z');
  assert.equal(banUntil('forever', now), '');
  assert.equal(banUntil('date', now, ''), '');
  assert.notEqual(banUntil('date', now, '2026-12-31T23:30'), '');
});

test('banEnd says until when a page is blocked', () => {
  assert.equal(banEnd({ reason: 'спам', until: null }), 'навсегда');
  assert.match(banEnd({ reason: 'спам', until: '2026-11-19T12:00:00Z' }), /^до 19 ноября 2026 в \d+:00$/);
});

test('rubles and the rate of a vote', () => {
  assert.equal(rubles(700), '7 ₽');
  assert.equal(rubles(750), '7,50 ₽');
  assert.equal(rubles(5), '0,05 ₽');
  assert.equal(rubles(125000), '1 250 ₽');
  assert.equal(rubles(100_000_000), '1 000 000 ₽');
  assert.equal(voteRate(700), '1 голос = 7 ₽');
});

test('stickerPackPrice: the foot of a card of the shop', async () => {
  const { stickerPackPrice } = await import('../static/labels.js');
  assert.deepEqual(stickerPackPrice({ price: 0 }), { text: 'Бесплатно', kind: 'free' });
  assert.deepEqual(stickerPackPrice({ price: 5 }), { text: '5 голосов', kind: 'paid' });
  assert.deepEqual(stickerPackPrice({ price: 5, bought: true }), { text: 'Добавить', kind: 'free' });
  assert.deepEqual(stickerPackPrice({ price: 5, bought: true, added: true }), { text: 'Приобретён', kind: 'purchased' });
});

test('shopPacks: the sections of the shop and its search', async () => {
  const { shopPacks } = await import('../static/labels.js');
  const packs = [
    { id: 'smiles', price: 0, holders: 1 },
    { id: 'cats', price: 2, holders: 5 },
    { id: 'bears', price: 4, holders: 5 },
    { id: '0198', name: 'Мой кот', price: 3, holders: 9, author: { name: 'Анна', surname: 'Иванова' } },
    { id: '0199', name: 'Собаки', about: 'гав', price: 0 },
  ];
  const ids = (list) => list.map((p) => p.id);

  assert.deepEqual(ids(shopPacks(packs, 'popular')), ['0198', 'cats', 'bears']);
  assert.deepEqual(ids(shopPacks(packs, 'free')), ['smiles', '0199']);
  assert.deepEqual(ids(shopPacks(packs, 'all')), ['smiles', 'cats', 'bears', '0198', '0199']);
  assert.deepEqual(ids(shopPacks(packs, 'all', 'КОТ')), ['cats', '0198']);
  assert.deepEqual(ids(shopPacks(packs, 'all', 'анна')), ['0198']);
  assert.deepEqual(ids(shopPacks(packs, 'free', 'гав')), ['0199']);
});

test('stickersCount and installsCount', async () => {
  const { stickersCount, installsCount } = await import('../static/labels.js');
  assert.equal(stickersCount(1), '1 стикер');
  assert.equal(stickersCount(3), '3 стикера');
  assert.equal(stickersCount(8), '8 стикеров');
  assert.equal(installsCount(0), '0 установок');
  assert.equal(installsCount(21), '21 установка');
});

test('the label of a foreign agent names the person', () => {
  assert.equal(foreignAgentText({ name: 'Анна', surname: 'Иванова', gender: 'female' }),
    'Данное сообщение (материал) создано и (или) распространено иностранным агентом Анной Ивановой либо касается деятельности иностранного агента Анны Ивановой.');
  assert.equal(foreignAgentText({ name: 'Пётр', surname: 'Петров', gender: 'male' }),
    'Данное сообщение (материал) создано и (или) распространено иностранным агентом Петром Петровым либо касается деятельности иностранного агента Петра Петрова.');
});

test('adDue: an ad after the third post, then after every eighth', async () => {
  const { adDue } = await import('../static/labels.js');
  const due = Array.from({ length: 30 }, (_, i) => i + 1).filter(adDue);
  assert.deepEqual(due, [3, 11, 19, 27]);
  assert.equal(adDue(0), false);
});

test('the stats of an ad are told as the admin panel says them', () => {
  assert.equal(adRate({ views: 0, clicks: 0 }), '—');
  assert.equal(adRate({ views: 1000, clicks: 42 }), '4,2 %');
  assert.equal(adRate({ views: 3, clicks: 1 }), '33,3 %');
  assert.equal(adRate({ views: 5, clicks: 5 }), '100 %');
  assert.equal(peopleCount(1), '1 человек');
  assert.equal(peopleCount(3), '3 человека');
  assert.equal(peopleCount(11), '11 человек');
});

test('the greeting under the fir tree is of the year to come, then of the year come', () => {
  assert.equal(newYearGreeting(new Date(2026, 11, 20)), 'С наступающим Новым годом!');
  assert.equal(newYearGreeting(new Date(2026, 9, 20)), 'С наступающим Новым годом!');
  assert.equal(newYearGreeting(new Date(2027, 0, 1)), 'С Новым годом!');
  assert.equal(newYearGreeting(new Date(2027, 0, 14)), 'С Новым годом!');
});

test('casino: chances, return and payout', () => {
  const prizes = [{ votes: 0, weight: 50 }, { votes: 5, weight: 25 }, { votes: 10, weight: 12 }, { votes: 20, weight: 8 }, { votes: 50, weight: 4 }, { votes: 100, weight: 1 }];

  assert.deepEqual(casinoChances(prizes), [50, 25, 12, 8, 4, 1]);
  assert.deepEqual(casinoChances([{ votes: 5, weight: 0 }, { votes: 9, weight: '' }]), [0, 0]);
  assert.equal(casinoChance(12.345), '12,3 %');
  assert.equal(casinoChance(0.0005), '0,0005 %');
  assert.equal(casinoChance(0.0000083), '0,0000083 %');
  assert.equal(casinoChance(0.04), '0,04 %');
  assert.equal(casinoChance(0), '0 %');
  assert.equal(casinoReturn(10, prizes), '70,5 %');
  assert.equal(casinoReturn(0, prizes), '—');
  assert.equal(casinoReturn(10, [{ votes: 5, weight: 0 }]), '—');
  assert.equal(casinoPayout({ spent: 40, won: 30 }), '75 %');
  assert.equal(casinoPayout({ spent: 0, won: 0 }), '—');
  assert.equal(casinoPrize(0), 'Пусто');
  assert.equal(casinoPrize(50), '50 голосов');
  assert.equal(spinsCount(1), '1 прокрутка');
  assert.equal(spinsCount(3), '3 прокрутки');
  assert.equal(spinsCount(11), '11 прокруток');
});

test('casino: bets', () => {
  assert.equal(wagersCount(1), '1 ставка');
  assert.equal(wagersCount(2), '2 ставки');
  assert.equal(wagersCount(5), '5 ставок');
  assert.equal(betMultiplier(150), '×1,5');
  assert.equal(betMultiplier(1000), '×10');
  // What a multiplier gives back on average with its chance.
  assert.equal(aimReturn(200, 4750), 95);
  assert.equal(aimReturn(1000, 900), 90);
  assert.equal(betOddsField(250), '2,5');
  assert.equal(betOddsField(1000), '10');
  assert.equal(typedOdds('2,5'), 250);
  assert.equal(typedOdds(' 1.01 '), 101);
  assert.equal(typedOdds('abc'), 0);
  assert.equal(betChance(4750), '47,5 %');
  assert.equal(betChance(6333), '63,3 %');
  assert.equal(betRoll(3721), '37,21');
  assert.equal(betRoll(5), '0,05');
  assert.equal(betPrize(11, 150), 16);
  assert.equal(betPrize(50, 200), 100);

  const limits = { least: 10, most: 100, balance: 70 };
  assert.equal(betAmount(40, 'least', limits), 10);
  assert.equal(betAmount(40, 'most', limits), 70);
  assert.equal(betAmount(40, 'most', { least: 10, most: 100, balance: 3 }), 10);
  assert.equal(betAmount(40, 'most', { least: 10, most: 100 }), 100);
  assert.equal(betAmount(15, 'half', limits), 10);
  assert.equal(betAmount(40, 'double', limits), 70);
  assert.equal(betAmount(40, 'double', { least: 10, most: 100, balance: 500 }), 80);
  assert.equal(betAmount(500, '', limits), 100);
  assert.equal(betAmount(Number.NaN, '', limits), 10);
});

test('casino: the highlight runs whole rounds and stops on the prize', () => {
  const plan = casinoPlan(6, 0, 4, 2);

  assert.equal(plan.length, 2 * 6 + 4);
  assert.equal(plan[0].cell, 1);
  assert.equal(plan.at(-1).cell, 4);
  assert.ok(plan.every((step, i) => i === 0 || step.delay >= plan[i - 1].delay), 'it only slows down');

  // Stopping where it stands takes a whole round more.
  assert.equal(casinoPlan(6, 3, 3, 1).length, 12);
  assert.equal(casinoPlan(6, 3, 3, 1).at(-1).cell, 3);
  assert.deepEqual(casinoPlan(0, 0, 0), []);
});

test('aimZone is the green part of the strip in its middle, as the user service counts it', () => {
  assert.deepEqual(aimZone(4750), { low: 2625, high: 7375 });
  assert.deepEqual(aimZone(10000), { low: 0, high: 10000 });
  assert.deepEqual(aimZone(951), { low: 4524, high: 5475 });
});

test('betEventShare and betEventOdds say what an outcome of an event holds and would bring', () => {
  assert.equal(betEventShare(30, 120), 25);
  assert.equal(betEventShare(0, 0), 0);
  // Without odds, all staked shared by those on the outcome.
  assert.equal(betEventOdds({ staked: 40 }, 100), '×2,5');
  assert.equal(betEventOdds({ staked: 30, odds: 0 }, 100), '×3,33');
  assert.equal(betEventOdds({ staked: 0 }, 100), '');
  // With odds, the odds, whatever is staked.
  assert.equal(betEventOdds({ staked: 0, odds: 250 }, 100), '×2,5');
  assert.equal(betEventOdds({ staked: 90, odds: 120 }, 100), '×1,2');
});

test('betEventState says where an event stands', () => {
  const outcomes = [{ name: 'Да' }, { name: 'Нет' }];

  assert.equal(betEventState({ state: 'resolved', winner: 1, outcomes }), 'Итог: Нет.');
  assert.equal(betEventState({ state: 'cancelled', outcomes }), 'Событие отменено, ставки вернулись.');
  assert.equal(betEventState({ state: 'open', open: false, outcomes }), 'Ставки больше не принимаются — ждём итога.');
  assert.equal(betEventState({ state: 'closed', open: false, outcomes }), 'Ставки больше не принимаются — ждём итога.');
  assert.equal(betEventState({ state: 'open', open: true, closesAt: null, outcomes }), 'Ставки принимаются, пока событие не закроют.');
  assert.match(betEventState({ state: 'open', open: true, closesAt: '2026-11-04T09:00:00Z', outcomes }), /^Ставки принимаются до 4 ноября 2026 в \d+:00\.$/);
});

test('autoplayNext plays the rounds chosen while the votes pay for them', () => {
  assert.equal(autoplayNext({ played: 0, times: 10, balance: 10, cost: 10 }), 'go');
  assert.equal(autoplayNext({ played: 10, times: 10, balance: 100, cost: 10 }), 'done');
  assert.equal(autoplayNext({ played: 3, times: 10, balance: 9, cost: 10 }), 'votes');
  assert.equal(autoplayNext({ played: 500, times: 0, balance: 10, cost: 10 }), 'go');
  assert.equal(autoplayNext({ played: 500, times: 0, balance: 0, cost: 10 }), 'votes');
});

test('votesFontSize makes a large balance fit its box', () => {
  assert.equal(votesFontSize(0), 48);
  assert.equal(votesFontSize(12345), 39);
  assert.equal(votesFontSize(1234567), 27);
  assert.equal(votesFontSize(1234567890), 18);
  assert.equal(votesFontSize(10 ** 20), 48);
});

test('the endless votes of the administrators are shown as ∞', () => {
  assert.equal(groupedVotes(10 ** 15), '∞');
  assert.equal(groupedVotes(10 ** 15 - 5000), '∞');
  assert.equal(groupedVotes(10 ** 14 - 1), '99\u00a0999\u00a0999\u00a0999\u00a0999');
  assert.equal(votesAmount(10 ** 15), '∞ голосов');
  assert.equal(votesFontSize(10 ** 15), 48);
  assert.equal(topValue('rich', 10 ** 15), '∞ голосов');
});

test('the statement of votes speaks of each line', () => {
  assert.equal(groupedVotes(1250000), '1\u00a0250\u00a0000');
  assert.equal(votesAmount(100000000), '100\u00a0000\u00a0000 голосов');
  assert.equal(votesAmount(-21), '21 голос');
  assert.equal(signedVotes(1500), '+1\u00a0500');
  assert.equal(signedVotes(-3), '−3');

  const ann = { id: 'a' };
  const bob = { id: 'b' };
  assert.deepEqual(voteOperation({ kind: 'transfer', amount: -4, peer: bob }, 'a'), { title: 'Перевод голосов', role: 'Получатель:' });
  assert.deepEqual(voteOperation({ kind: 'transfer', amount: 4, peer: bob }, 'a'), { title: 'Поступление голосов', role: 'Отправитель:' });
  assert.deepEqual(voteOperation({ kind: 'voucher', amount: 10 }, 'a'), { title: 'Активация ваучера', role: '' });
  assert.deepEqual(voteOperation({ kind: 'fee', amount: -1, peer: bob }, 'a'), { title: 'Комиссия за перевод', role: 'Получатель:' });
  assert.deepEqual(voteOperation({ kind: 'tax', amount: -1, note: 'cats', peer: bob }, 'a'), { title: 'Налог с продажи набора «Котики»', role: 'Покупатель:' });
  assert.deepEqual(voteOperation({ kind: 'rating', amount: -15, note: '5' }, 'a'), { title: 'Повышение рейтинга на 5 %', role: '' });
  assert.deepEqual(voteOperation({ kind: 'gift', amount: -1, peer: bob, note: 'freenet_cup' }, 'a'), { title: 'Подарок «Кубок»', role: 'Получатель:' });
  assert.deepEqual(voteOperation({ kind: 'gift', amount: -1, peer: ann, note: 'freenet_coin' }, 'a'), { title: 'Покупка подарка «Золотая монета»', role: '' });
  assert.equal(voteOperation({ kind: 'stickers', amount: -2, note: 'cats' }, 'a').title, 'Набор стикеров «Котики»');
  assert.deepEqual(voteOperation({ kind: 'stickers', amount: 2, peer: bob, note: 'Мои коты' }, 'a'), { title: 'Продажа набора стикеров «Мои коты»', role: 'Покупатель:' });
  assert.equal(voteOperation({ kind: 'casino', amount: -10 }, 'a').title, 'Казино: прокрутка «Огонька»');
  assert.equal(voteOperation({ kind: 'aim', amount: 95 }, 'a').title, 'Казино: выигрыш в «Прицеле»');
  assert.equal(voteOperation({ kind: 'stake', amount: -5, note: 'Пойдёт снег?' }, 'a').title, 'Ставка на событие «Пойдёт снег?»');
  assert.equal(voteOperation({ kind: 'stake', amount: 9, note: 'Пойдёт снег?' }, 'a').title, 'Выплата по событию «Пойдёт снег?»');
  assert.equal(voteOperation({ kind: 'new', amount: 1 }, 'a').title, 'Зачисление голосов');

  for (const address of ['ann', '@ann', ' https://freenet19.fun/ann ', 'https://freenet19.fun/ann/', 'https://freenet19.fun/votes?act=transfer&to=ann']) {
    assert.equal(transferNickname(address), 'ann', address);
  }
  assert.equal(transferURL('https://freenet19.fun', 'ann'), 'https://freenet19.fun/votes?act=transfer&to=ann');
});

test('the rocket grows as the user service counts it', () => {
  // The same cases as TestRocketMultiplier of the user service.
  assert.equal(rocketMultiplier(-1000), 100);
  assert.equal(rocketMultiplier(0), 100);
  assert.equal(rocketMultiplier(1000), 112);
  assert.equal(rocketMultiplier(5776), 199);
  assert.equal(rocketMultiplier(5777), 200);
  assert.equal(rocketMultiplier(5777.9), 200);
  assert.equal(rocketMultiplier(3_600_000), 1_000_000);

  assert.equal(rocketTime(100), 0);
  assert.equal(rocketMultiplier(Math.ceil(rocketTime(237))), 237);
  assert.ok(rocketMultiplier(Math.ceil(rocketTime(237)) - 2) < 237);

  assert.equal(rocketShown(100), '×1,00');
  assert.equal(rocketShown(237), '×2,37');
  assert.equal(rocketShown(100000), '×1000,00');
  assert.equal(rocketsCount(1), '1 полёт');
  assert.equal(rocketsCount(3), '3 полёта');
  assert.equal(rocketsCount(5), '5 полётов');

  assert.deepEqual(rocketScale(0), { time: 8000, top: 200 });
  assert.ok(rocketScale(20000).time > 20000 && rocketScale(20000).top > rocketMultiplier(20000));
});

test('a rocket line of the statement', () => {
  assert.equal(voteOperation({ kind: 'rocket', amount: -10 }).title, 'Казино: ставка в «Ракетке»');
  assert.equal(voteOperation({ kind: 'rocket', amount: 20 }).title, 'Казино: выигрыш в «Ракетке»');
});

test('the fees of the economy', () => {
  // 3% on top of a transfer, rounded up, at least 1 — as the user service.
  assert.equal(transferFee(0), 0);
  assert.equal(transferFee(1), 1);
  assert.equal(transferFee(33), 1);
  assert.equal(transferFee(34), 2);
  assert.equal(transferFee(100), 3);
  assert.equal(transferFee(101), 4);
  // The administrators set the percent: 0 takes nothing.
  assert.equal(transferFee(100, 0), 0);
  assert.equal(transferFee(1000, 5), 50);

  // The author gets 90% of a sale, rounded down.
  assert.equal(stickerIncome(1), 0);
  assert.equal(stickerIncome(3), 2);
  assert.equal(stickerIncome(10), 9);
  assert.equal(stickerIncome(55), 49);
  assert.equal(stickerIncome(100, 5), 95);
  assert.equal(stickerIncome(100, 0), 100);
});

test('adminEconomy: the words under the treasury and the votes of the pages', () => {
  const said = adminEconomy({ votes: 21, outstanding: 0, held: 1, bonus: 10 });
  assert.equal(said.treasury, 'голос — сальдо Казны: собрано минус выплачено');
  assert.equal(said.held, 'голос на счетах людей');
  assert.equal(said.bonus, 'голосов — ежедневный бонус');
  assert.equal(said.maxWin, undefined);
  assert.equal(adminEconomy({ votes: -2, bonus: 2 }).treasury, 'голоса — сальдо Казны: собрано минус выплачено');
  assert.equal(adminEconomy({ votes: 5, bonus: 2 }).held, 'голосов на счетах людей');
});

test('Prime is sold by the month, the quarter and the year', () => {
  assert.deepEqual(primePeriods.map(([v, , months]) => [v, months]), [['month', 1], ['quarter', 3], ['year', 12]]);
  const prices = { month: 150, quarter: 400, year: 1500 };
  assert.equal(primeSaving(prices, 1, 150), 0);
  assert.equal(primeSaving(prices, 3, 400), 11);
  assert.equal(primeSaving(prices, 12, 1500), 16);
  assert.equal(primeSaving(prices, 3, 900), 0);
  assert.deepEqual(voteOperation({ kind: 'prime', amount: -150, note: 'month' }, 'a'), { title: 'Prime на месяц', role: '' });
  assert.deepEqual(voteOperation({ kind: 'prime', amount: -400, note: 'quarter', peer: { id: 'b' } }, 'a'), { title: 'Prime на квартал', role: 'Получатель:' });
  assert.equal(notificationVerb('prime', { gender: 'female' }), 'подарила');
  assert.equal(errorText({ message: 'this is for freenet prime only' }), 'Это доступно только с Prime.');
  assert.deepEqual(primeTerms, { month: 'месяц', quarter: 'квартал', year: 'год' });
});

test('the titles of a page are grouped, the latest first', () => {
  assert.deepEqual(titleIcons.map(([icon]) => icon), ['crown', 'cup', 'star', 'laurel', 'shield', 'flame', 'diamond', 'lightning']);
  for (const [icon] of titleIcons) assert.equal(titleIconPath(icon), `/static/titles/${icon}.svg`);
  assert.equal(titleIconPath('skull'), '/static/titles/star.svg');

  const groups = titleGroups([
    { id: 'c', name: 'Король шахты', icon: 'crown', grantedAt: '2026-10-26T10:00:00Z' },
    { id: 'b', name: 'Игрок недели', icon: 'cup', grantedAt: '2026-10-25T10:00:00Z' },
    { id: 'a', name: 'Король шахты', icon: 'crown', grantedAt: '2026-10-20T10:00:00Z' },
    { id: 'd', name: 'Король шахты', icon: 'star', grantedAt: '2026-10-21T10:00:00Z' },
  ]);
  assert.deepEqual(groups.map((g) => [g.name, g.icon, g.count, g.last]), [
    ['Король шахты', 'crown', 2, '2026-10-26T10:00:00Z'],
    ['Игрок недели', 'cup', 1, '2026-10-25T10:00:00Z'],
    ['Король шахты', 'star', 1, '2026-10-21T10:00:00Z'],
  ]);
  assert.deepEqual(titleGroups([]), []);
});

test('collectible gifts: the traits of the domain, their pictures, fees and statements', async () => {
  const { giftModels, giftBackdrops, giftSymbols, marketSorts, collectibleImage, collectibleName, backdropGradient, traitChance, giftIncome, auctionLeft, editionLeft, giftHistoryLine, voteOperation, errorText } = await import('../static/labels.js');

  // The traits are the user service's, in its order, and a select offers
  // them in lower case.
  const source = readFileSync(new URL('../../user/internal/user/collectible.go', import.meta.url), 'utf8');
  const traits = (name) => [...new RegExp(`${name} = \\[\\]GiftTrait\\{([\\s\\S]*?)\\n\\t\\}`).exec(source)[1].matchAll(/\{"(\w+)", \d+\}/g)].map((m) => m[1]);
  assert.deepEqual(giftModels.map(([m]) => m), traits('giftModels'));
  assert.deepEqual(giftBackdrops.map(([b]) => b), traits('giftBackdrops'));
  assert.deepEqual(giftSymbols.map(([s]) => s), traits('giftSymbols'));
  for (const [, text] of [...giftModels, ...giftBackdrops, ...giftSymbols, ...marketSorts]) assert.equal(text[0], text[0].toLowerCase(), text);
  for (const [backdrop] of giftBackdrops) assert.match(backdropGradient(backdrop), /^radial-gradient/);

  // Every gift has a picture in every model, drawn by scripts/giftgen.
  for (const gift of ['freenet_coin', 'prime_star']) {
    for (const [model] of giftModels) {
      const path = collectibleImage(gift, model);
      assert.equal(path, `/static/gifts/models/${gift}_${model}.png`);
      assert.ok(existsSync(new URL(`..${path}`, import.meta.url)), path);
    }
  }
  assert.equal(collectibleImage('rose', 'onyx'), '/static/gifts/unknown.svg');
  assert.equal(collectibleName({ gift: 'freenet_coin', number: 7 }), 'Золотая монета #7');

  assert.equal(traitChance([{ name: 'onyx', weight: 100 }], 'onyx'), '10 %');
  assert.equal(traitChance([{ name: 'black', weight: 5 }], 'black'), '0,5 %');
  assert.equal(traitChance([], 'onyx'), '');

  // The fee is rounded up, as the service takes it.
  assert.equal(giftIncome(100), 95);
  assert.equal(giftIncome(100, true), 97);
  assert.equal(giftIncome(1), 0);

  const now = Date.parse('2026-10-22T12:00:00Z');
  assert.equal(auctionLeft('2026-10-22T12:00:30Z', now), 'заканчивается');
  assert.equal(auctionLeft('2026-10-22T12:07:00Z', now), '7 мин');
  assert.equal(auctionLeft('2026-10-22T14:15:00Z', now), '2 ч 15 мин');
  assert.equal(auctionLeft('2026-10-24T15:00:00Z', now), '2 д 3 ч');

  assert.equal(editionLeft(0, 5), '');
  assert.equal(editionLeft(500, 380), 'осталось 120 из 500');
  assert.equal(editionLeft(500, 500), 'распродан');
  assert.equal(giftHistoryLine('auction'), 'Продан на аукционе');

  assert.equal(voteOperation({ kind: 'gift_upgrade', amount: -50, note: 'freenet_coin' }).title, 'Улучшение подарка «Золотая монета»');
  assert.deepEqual(voteOperation({ kind: 'gift_sale', amount: 95, note: 'freenet_coin#7' }), { title: 'Продажа подарка «Золотая монета» #7', role: 'Покупатель:' });
  assert.equal(voteOperation({ kind: 'gift_sale', amount: -100, note: 'freenet_coin#7' }).title, 'Покупка подарка «Золотая монета» #7');
  assert.equal(voteOperation({ kind: 'gift_refund', amount: 20, note: 'prime_star#1' }).title, 'Возврат ставки: «Звезда Prime» #1');
  assert.equal(errorText({ message: 'the bid is too low' }), 'Ставка слишком мала.');
});

test('the mine counts the energy on, sells ore and logs digs', async () => {
  const { mineEnergy, energyWait, oreValue, oreName, mineDug, mineSold, mineChance, typedMineChance, ores } = await import('../static/labels.js');
  const settings = { energy: 10, regenSeconds: 900, events: { nuggetTimes: 3, drinkEnergy: 3 } };

  assert.deepEqual(mineEnergy({ energy: 10, nextEnergySeconds: 0 }, settings, 5000), { energy: 10, next: 0 });
  assert.deepEqual(mineEnergy({ energy: 8, nextEnergySeconds: 600 }, settings, 100), { energy: 8, next: 500 });
  assert.deepEqual(mineEnergy({ energy: 8, nextEnergySeconds: 600 }, settings, 700), { energy: 9, next: 800 });
  assert.deepEqual(mineEnergy({ energy: 8, nextEnergySeconds: 600 }, settings, 1500), { energy: 10, next: 0 });

  assert.equal(energyWait(245), '4:05');
  assert.equal(energyWait(3750), '1:02:30');
  assert.equal(energyWait(0), '0:00');

  assert.equal(oreValue(150, 2), 3);
  assert.equal(oreValue(49, 2), 0);
  assert.deepEqual(ores, ['stone', 'coal', 'iron', 'gold', 'diamond', 'crystal']);
  assert.equal(oreName('gold'), 'Золото');

  const dug = (event, units, votes = 0) => ({ event, units, votes, mine: { miner: { energy: 8, experience: 1250 } } });
  assert.equal(mineDug('stone', dug('', 147), settings), 'Камень: +147 шт. Энергия: 8, опыт: 1 250.');
  assert.match(mineDug('gold', dug('nugget', 150), settings), /^Самородок! Руды в 3 раза больше\. Золото: \+150 шт\./);
  assert.match(mineDug('stone', dug('chest', 150, 25), settings), /^Вы нашли сундук: \+25 голосов!/);
  assert.match(mineDug('stone', dug('collapse', 0), settings), /^Обвал!/);
  assert.match(mineDug('stone', dug('drink', 120), settings), /^Вы нашли энергетик: \+3 энергии\./);
  assert.equal(mineSold({ units: 1200, votes: 24 }), 'Продано 1 200 шт. руды за 24 голоса.');

  assert.equal(mineChance(300), '3');
  assert.equal(mineChance(50), '0,5');
  assert.equal(typedMineChance('0,5'), 50);
  assert.equal(typedMineChance(' 3 '), 300);
});

test('the ores of the site are those of the user service', () => {
  const source = readFileSync(new URL('../../user/internal/user/mine.go', import.meta.url), 'utf8');
  const order = source.match(/var Ores = \[\]Ore\{([^}]*)\}/)[1].split(',').map((name) => name.trim());
  const values = Object.fromEntries([...source.matchAll(/(Ore\w+)\s+Ore = "(\w+)"/g)].map(([, name, value]) => [name, value]));
  return import('../static/labels.js').then(({ ores }) => assert.deepEqual(order.map((name) => values[name]), ores));
});

test('the styles of Prime of the site are those of the user service', async () => {
  const source = readFileSync(new URL('../../user/internal/user/prime_style.go', import.meta.url), 'utf8');
  const list = (name) => JSON.parse('[' + source.match(new RegExp(name + ' += \\[\\]\\w+\\{([^}]*)\\}'))[1] + ']');
  const labels = await import('../static/labels.js');
  const values = (choices) => choices.map(([value]) => value);

  assert.deepEqual(values(labels.primeNames), list('PrimeNames'));
  assert.deepEqual(values(labels.primeFrames), list('PrimeFrames'));
  assert.deepEqual(values(labels.primeBackgrounds), list('PrimeBackgrounds'));
  assert.deepEqual(values(labels.primeMoods), list('PrimeMoods'));
});

test('the classes and the mood of a page dressed with Prime', async () => {
  const { primeMoodText, primeStyleClasses, primeNameClass, primeDress } = await import('../static/labels.js');

  assert.deepEqual(primeStyleClasses(null), []);
  assert.deepEqual(primeStyleClasses({ name: 'ruby', frame: 'stars', background: '', mood: '' }), ['prime-frame-stars']);
  assert.deepEqual(primeStyleClasses({ name: '', frame: '', background: 'clouds' }), ['prime-frame-amethyst', 'prime-bg', 'prime-bg-clouds']);

  assert.equal(primeNameClass({ name: 'Ann' }), '');
  assert.equal(primeNameClass({ prime: true }), 'prime-name');
  assert.equal(primeNameClass({ prime: true, primeStyle: { name: 'amethyst' } }), 'prime-name');
  assert.equal(primeNameClass({ prime: true, primeStyle: { name: 'rainbow' } }), 'prime-name prime-name-rainbow');
  assert.deepEqual(primeDress({ prime: true, primeStyle: { background: 'stars' } }), ['prime-dressed', 'prime-bg-stars']);
  assert.deepEqual(primeDress({ prime: true, primeStyle: { background: '' } }), []);
  assert.deepEqual(primeDress({ primeStyle: { background: 'stars' } }), []);
  assert.equal(primeMoodText('love'), 'Настроение: влюблённое');
  assert.equal(primeMoodText(''), '');
  assert.equal(primeMoodText('bored'), '');
});

test('the pets and the boards of the site are those of the user service', async () => {
  const labels = await import('../static/labels.js');
  const listed = (file, list, type) => {
    const source = readFileSync(new URL('../../user/internal/user/' + file, import.meta.url), 'utf8');
    const order = source.match(new RegExp(`var ${list} = \\[\\]${type}\\{([^}]*)\\}`))[1].split(',').map((name) => name.trim());
    const values = Object.fromEntries([...source.matchAll(new RegExp(`(\\w+)\\s+${type} = "(\\w+)"`, 'g'))].map(([, name, value]) => [name, value]));
    return order.map((name) => values[name]);
  };

  assert.deepEqual(labels.petKinds, listed('pet.go', 'PetKinds', 'PetKind'));
  assert.deepEqual(labels.topBoards, listed('top.go', 'TopBoards', 'TopBoard'));
  for (const kind of labels.petKinds) assert.notEqual(labels.petKindName(kind), kind);
  for (const board of labels.topBoards) assert.notEqual(labels.topBoardName(board), board);
});

test('pets, fights, boards and guests are told', async () => {
  const { petFightResult, petLevelProgress, petMood, petWait, petHitText, guestCame } = await import('../static/labels.js');

  assert.deepEqual(petFightResult({ won: true, rating: 16 }, true), { won: true, text: 'Победа', rating: '+16' });
  assert.deepEqual(petFightResult({ won: true, rating: 16 }, false), { won: false, text: 'Поражение', rating: '−16' });
  assert.equal(petFightResult({ won: false, rating: 0 }, true).rating, '±0');

  assert.equal(petLevelProgress({ experience: 150, levelExperience: 100, nextLevelExperience: 300 }), 25);
  assert.equal(petLevelProgress({ experience: 900, levelExperience: 900, nextLevelExperience: 900 }), 100);
  assert.equal(petMood({ satiety: 90, joy: 80 }), 'Счастлив');
  assert.equal(petMood({ satiety: 10, joy: 80 }), 'Голоден');
  assert.equal(petMood({ satiety: 50, joy: 10 }), 'Скучает');
  assert.equal(petWait(61), '2 мин.');
  assert.equal(petWait(40.2), '41 с');

  const cat = { name: 'Мурка' };
  const dog = { name: 'Рекс' };
  assert.equal(petHitText({ byAttacker: true, damage: 5 }, cat, dog), 'Мурка бьёт. Рекс: −5.');
  assert.equal(petHitText({ byAttacker: false, dodged: true }, cat, dog), 'Рекс бьёт, но Мурка уворачивается.');

  assert.equal(topValue('pets', 1016), 'рейтинг 1\u00a0016');
  assert.match(topValue('rich', 3000), /^3\s000\sголосов$/);
  assert.equal(guestCame({ user: { gender: 'female' } }), ' заходила на вашу страницу.');
  assert.equal(guestCame({ user: {} }), ' заходил на вашу страницу.');
});
