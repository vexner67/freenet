import { api, currentUser, started } from './api.js';
import { isBirthdayToday, relationships, partnerName, partnerPrefix, takesPartner, worldviews, politicalViews, label, clubsCount, completenessHint, presence, formatBirthday, formatDateTime, limitedAccess, accusativeName, banEnd, genitiveName, primeMoodIcons, primeMoodText, primeNameClass, primeStyleClasses, audioArtist, audioTitle, audiosCount, plural, MAX_RATING, MAX_BOUGHT_RATING, ratingPercent, ratingTier, balanceAmount, votesAmount, titleGroups, titleIconPath } from './labels.js';
import { $, el, fullName, hideError, logout, pagePath, renderChrome, showError, verifiedMark } from './layout.js';
import { giftsBlock, pickPath } from './gifts.js';
import { initBlacklistLink } from './blacklist.js';
import { initIgnoreLink } from './feedsettings.js';
import { initFollow } from './follow.js';
import { initPhoto } from './photo.js';
import { openSendBox } from './sendbox.js';
import { button, openModal } from './modal.js';
import { initWall } from './wall.js';
import { showBackdrop } from './backdrop.js';
import { personCell } from './people.js';
import { openStories } from './stories.js';
import { addButton, audioNode, play } from './audio.js';
import { tr } from './i18n.js';

const nickname = decodeURIComponent(location.pathname.slice(1));
const CLUBS_SHOWN = 6;

// rows lists the table under the name, in openvk's order; private marks
// rows others cannot see (only the owner gets them at all, from /me).
function rows(user, privacy) {
  return [
    [tr('Семейное положение:'), relationship(user), privacy && !privacy.showRelationship],
    [tr('Дата регистрации:'), user.registeredAt ? formatDateTime(user.registeredAt) : '', false],
    [tr('Родной город:'), user.hometown && searchLink('hometown', user.hometown), false],
    [tr('Полит. взгляды:'), label(politicalViews, user.politicalViews), privacy && !privacy.showPoliticalViews],
    [tr('Мировоззрение:'), label(worldviews, user.worldview), privacy && !privacy.showWorldview],
    [tr('Дата рождения:'), user.birthday ? birthdayText(user) : '', privacy && !privacy.showBirthday],
  ].filter(([, value]) => value);
}

// relationship is "Замужем за Петром Петровым" as openvk says it: the
// status, then whom it names, a link.
function relationship(user) {
  const status = label(relationships(user.gender), user.relationship);
  if (!status || !user.partner || !takesPartner(user.relationship)) return status;

  return el('span', {},
    status + ' ' + partnerPrefix(user.relationship, user.gender) + ' ',
    el('a', { href: pagePath(user.partner), textContent: partnerName(user.relationship, user.gender, user.partner) }));
}

// birthdayText is the date; the owner who shows only the day and the month
// sees the year too, and that the others do not.
function birthdayText(user) {
  const text = formatBirthday(user.birthday);
  return user.hideBirthYear ? text + tr(' (год видите только вы)') : text;
}

// searchLink finds the people sharing a value, as openvk links them.
function searchLink(filter, value) {
  return el('a', { href: '/search?' + new URLSearchParams({ [filter]: value }), textContent: value });
}

// linkedList splits a favourite field at commas into such links.
function linkedList(filter, text) {
  const items = text.split(',').map((s) => s.trim()).filter(Boolean);
  return el('span', {}, ...items.flatMap((item, i) => [i > 0 ? ', ' : '', searchLink(filter, item)]));
}

function external(href, text) {
  return el('a', { href, textContent: text, target: '_blank', rel: 'noopener nofollow ugc' });
}

// renderAbout fills "Контактная информация" and "Личная информация", with
// "[ Редактировать ]" for the owner; the fields of "Дополнительно" go where
// their owner put them. It tells whether there is anything to show.
function renderAbout(user, isOwner) {
  const c = user.contacts;
  const i = user.interests;
  const extra = (place) => (user.fields ?? []).filter((f) => f.place === place).map((f) => [f.name + ':', f.text]);

  const contacts = [
    [tr('Электронная почта:'), c.email && external('mailto:' + c.email, c.email)],
    ['Telegram:', c.telegram && external('https://t.me/' + c.telegram, '@' + c.telegram)],
    [tr('Личный сайт:'), c.website && external(c.website, c.website)],
    [tr('Город:'), c.city && searchLink('city', c.city)],
    [tr('Адрес:'), c.address],
    ...extra('contacts'),
  ];
  const interests = [
    [tr('Интересы:'), i.interests],
    [tr('Любимая музыка:'), i.music],
    [tr('Любимые фильмы:'), i.films && linkedList('film', i.films)],
    [tr('Любимые ТВ-шоу:'), i.shows && linkedList('show', i.shows)],
    [tr('Любимые книги:'), i.books && linkedList('book', i.books)],
    [tr('Любимые цитаты:'), i.quotes],
    [tr('Любимые игры:'), i.games],
    ...extra('interests'),
    [tr('О себе:'), i.about],
  ];

  // Unlike openvk, an empty part is not shown with "Информация
  // отсутствует.", and an empty block not at all.
  let shown = false;
  for (const [id, list] of [['contacts', contacts], ['interests', interests]]) {
    const filled = list.filter(([, value]) => value);
    $(id).replaceChildren(...filled.map(([title, value]) => el('tr', {},
      el('th', { textContent: title }),
      el('td', { className: 'about-text' }, value))));
    $(id + '-part').hidden = filled.length === 0;
    $(id + '-edit').hidden = !isOwner;
    shown ||= filled.length > 0;
  }
  return shown;
}

// renderHeader is openvk's: the name, "(это вы)" to the owner, and on the
// right "Онлайн" or when the person was last online. The owner, with this
// page open, is online.
function renderHeader(profile, isOwner) {
  const name = fullName(profile);
  const seen = isOwner ? tr('Онлайн') : presence(profile);

  document.title = `${name} | Freenet`;
  // Prime: the name and the frame of the photo are amethyst, or
  // as the owner dressed the page ("Оформление страницы").
  document.body.classList.toggle('prime-page', Boolean(profile.prime));
  document.body.classList.add(...primeStyleClasses(profile.primeStyle));
  showMood(profile.primeStyle?.mood ?? '');
  showSparkles(profile.prime && profile.primeStyle?.frame === 'glitter');
  $('title').replaceChildren(profile.prime ? el('span', { className: primeNameClass(profile), textContent: name }) : name,
    verifiedMark(profile), ...(isOwner ? [' ', el('b', { textContent: tr('(это вы)') })] : []));
  $('title-aside').replaceChildren(isOwner || profile.online ? el('b', { textContent: seen }) : seen || '@' + profile.nickname);
}

// showBirthday dresses the page for the one day of its owner's birthday:
// a plaque, a party hat on the photo and its own background.
function showBirthday(user, isOwner) {
  if (!isBirthdayToday(user.birthday)) return;

  document.body.classList.add('birthday');
  $('birthday-title').textContent = isOwner ? tr('С днём рождения!') : tr('Сегодня день рождения у {0}!', genitiveName(user));
  $('birthday-text').textContent = isOwner ? tr('Желаем счастья!') : tr('Поздравьте и отправьте подарок.');
  $('birthday-hat').hidden = false;
  $('birthday').hidden = false;
}

// showWrite is "Отправить сообщение" under the photo: as on openvk, it
// opens a small window to write in; the address leads to the dialog for a
// new tab.
function showWrite(profile, me) {
  $('write').href = '/im/' + encodeURIComponent(profile.nickname);
  $('write').hidden = false;
  $('write').addEventListener('click', (event) => {
    if (event.ctrlKey || event.metaKey || event.shiftKey || event.button !== 0) return;
    event.preventDefault();
    openSendBox(profile, me);
  });
}

function render(user, isOwner, profile) {
  showBirthday(user, isOwner);
  const fullName = [user.name, user.surname].filter(Boolean).join(' ');

  renderHeader(profile, isOwner);
  $('name').textContent = fullName;

  showStatus(user.status, isOwner);
  if ($('room-presence')) {
    $('room-presence').hidden = !profile.room;
    $('room-presence').replaceChildren(...(profile.room ? [tr('в комнате: '), el('a', { href: `/room/${encodeURIComponent(profile.room.id)}`, textContent: profile.room.title })] : []));
  }

  const info = rows(user, isOwner ? user.privacy : null);
  $('info').replaceChildren(...info.map(([title, value, isPrivate]) => el('tr', {},
    el('th', { textContent: title }),
    el('td', {}, value, isPrivate && el('span', { className: 'private-mark', textContent: tr(' · видно только вам') })),
  )));
  const hasAbout = renderAbout(user, isOwner);

  // "Кому видно основную информацию моей страницы": openvk leaves the
  // names, the photo and the status.
  $('info').hidden = Boolean(user.infoHidden);
  $('about-block').hidden = Boolean(user.infoHidden) || !hasAbout;

  $('edit').hidden = !isOwner;
  $('profile').hidden = false;

  initWall(user, me);
}

// showMood is the smiley of the mood the owner with Prime chose ("" for
// none): on the right, under "Онлайн".
function showMood(mood) {
  const smiley = $('mood');
  smiley.hidden = !primeMoodIcons[mood];
  if (smiley.hidden) return;

  smiley.src = primeMoodIcons[mood];
  smiley.title = primeMoodText(mood);
}

// showSparkles blinks sparkles around the photo in the glitter frame.
function showSparkles(on) {
  const box = document.querySelector('.profile .avatar-box');
  box.querySelectorAll('.prime-sparkles').forEach((s) => s.remove());
  if (on) box.append(el('span', { className: 'prime-sparkles' }), el('span', { className: 'prime-sparkles late' }));
}

// showStatus is the status under the name; to the owner it is openvk's
// "[ изменить статус ]" when empty, and either opens the box to edit it.
function showStatus(text, isOwner) {
  const status = $('status');
  status.classList.toggle('empty', !text);
  status.classList.toggle('status-editable', isOwner);
  status.textContent = text || (isOwner ? tr('[ изменить статус ]') : '');
  status.onclick = isOwner ? () => editStatus(text) : null;
}

// showMusic is "слушает: …" under the status, for the song its owner
// plays now, and over the information the song of the page with the link
// to their music, "Аудиозаписи".
async function showMusic(profile, me, isOwner) {
  let music;
  try {
    music = await api.music(profile.nickname);
  } catch {
    return;
  }

  const now = music.listening;
  if (now) {
    $('listening').replaceChildren(
      el('span', { className: 'listening-icon' }),
      tr('слушает:'), ' ',
      el('a', { textContent: `${audioArtist(now)} — ${audioTitle(now)}`, onclick: () => play(now) }));
  }
  $('listening').hidden = !now;

  const box = $('page-audio');
  if (!music.pageAudio && !music.count) {
    box.hidden = true;
    return;
  }

  // openvk's block of a page: "Аудиозаписи", how many and "Все", then the
  // song of the page and the last songs added, each playing the rest.
  let songs = [];
  if (music.count) {
    try {
      songs = (await api.audios(profile.nickname)).audios ?? [];
    } catch {
      songs = [];
    }
  }
  const song = music.pageAudio;
  const list = [song, ...songs.filter((a) => a.id !== song?.id)].filter(Boolean).slice(0, 4);
  const all = '/audio/' + encodeURIComponent(profile.nickname);
  const links = [el('a', { href: all, textContent: tr('Все') })];
  if (song && isOwner) {
    links.unshift(el('a', {
      textContent: tr('Убрать песню страницы'),
      onclick: async () => {
        await api.setPageAudio('').catch(() => {});
        showMusic(profile, me, isOwner);
      },
    }), ' | ');
  }

  box.replaceChildren(
    el('div', { className: 'section-title', textContent: tr('Аудиозаписи') }),
    el('div', { className: 'subtitle' }, el('span', { textContent: audiosCount(music.count) }), el('span', {}, ...links)),
    el('div', { className: 'audio-list page-audio-list' }, ...list.map((a) => {
      const row = audioNode(a, { queue: () => list, tools: me && !isOwner && !a.added ? [addButton(a)] : [] });
      if (a.id === song?.id) row.classList.add('page-audio-song');
      return row;
    })));
  box.hidden = false;
}

// editStatus is openvk's box under the name: the status in a field and
// "Сохранить"; Escape leaves it as it was.
function editStatus(text) {
  const status = $('status');
  const input = el('input', { type: 'text', maxLength: 200, value: text, ariaLabel: tr('Статус') });
  const save = el('button', { className: 'button', type: 'submit', textContent: tr('Сохранить') });
  const form = el('form', { className: 'status-edit' }, input, save);

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError($('error'));
    save.disabled = true;

    try {
      const updated = await api.updateStatus(input.value.trim());
      Object.assign(me, updated);
      showStatus(updated.status, true);
      showCompleteness(updated, true);
    } catch (e) {
      save.disabled = false;
      showError($('error'), e);
    }
  });
  input.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') showStatus(text, true);
  });

  status.onclick = null;
  status.classList.remove('empty');
  status.replaceChildren(form);
  input.focus();
}

// showCompleteness is openvk's gauge of the rating of the page, to
// whoever is signed in, and to the owner the hints of what to fill in. Up
// to 100 % it fills as openvk's; past it the gauge is full and silver,
// from 200 % gold and from 1000 % ruby (ratingTier).
function showCompleteness(user, isOwner) {
  const percent = user.completeness ?? 0;
  // An empty page of another, or one that tells nothing, has no gauge.
  if (!me || (!percent && !isOwner)) return;

  const bar = el('div');
  bar.style.width = Math.min(percent, 100) + '%';

  const hints = isOwner
    ? (user.unfilledParts ?? []).map(completenessHint).filter(Boolean).map(([href, text], i) =>
      el('a', { className: 'completeness-hint hint-' + user.unfilledParts[i].part, href, textContent: text }))
    : [];
  const tier = ratingTier(percent);

  $('completeness').replaceChildren(
    el('div', { className: 'completeness-gauge' + (tier ? ' rating-' + tier : '') },
      bar, el('span', { textContent: ratingPercent(percent) })),
    ...(hints.length ? [el('div', { className: 'completeness-hints' }, ...hints)] : []),
  );
  $('completeness').hidden = false;
}

// showRaise is "Повысить рейтинг" under the photo, to whoever is signed
// in: one raises one's own page and another's for one's votes.
function showRaise(profile, isOwner) {
  if (!me) return;

  $('raise-rating').onclick = (event) => {
    event.preventDefault();
    raiseRating(profile, isOwner);
  };
  $('raise-rating').hidden = false;
}

// raiseRating is the window of "Повысить рейтинг": how many percent, a
// vote each, with the balance and the cost; the gauge shows the new
// rating at once.
function raiseRating(profile, isOwner) {
  const page = isOwner ? me : profile;
  const modal = openModal(tr('Повысить рейтинг'), { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const input = el('input', { type: 'number', className: 'modal-input', min: 1, step: 1, required: true, ariaLabel: tr('Сколько процентов') });
  const cost = el('div', { className: 'rating-cost' });
  const balance = el('span', { textContent: balanceAmount(me.votes ?? 0) });
  // Votes buy up to MAX_BOUGHT_RATING; the last 100 % are for the filled page.
  const left = () => Math.min(MAX_RATING - (page.completeness ?? 0), MAX_BOUGHT_RATING - (page.rating ?? 0));
  input.max = Math.max(left(), 1);

  const showCost = () => {
    const n = Number(input.value);
    cost.textContent = Number.isInteger(n) && n > 0 ? tr('Стоимость: {0}.', votesAmount(n)) : '\u00a0';
  };
  input.addEventListener('input', showCost);
  showCost();

  modal.body.append(
    el('p', { textContent: isOwner
      ? tr('Рейтинг вашей страницы: {0}.', ratingPercent(page.completeness ?? 0))
      : tr('Рейтинг страницы {0}: {1}.', genitiveName(profile), ratingPercent(page.completeness ?? 0)) }),
    el('p', { className: 'rating-balance' }, tr('Ваш баланс: '), balance),
    el('label', { className: 'rating-field' }, tr('Сколько процентов:'), input),
    cost,
    el('p', { className: 'hint', textContent: tr('Один процент — один голос. Купить можно до {0}, остальные 100 % — за заполненную страницу.', ratingPercent(MAX_BOUGHT_RATING)) }),
    error,
  );
  const submit = button(tr('Повысить'), null, { type: 'submit' });
  modal.actions.append(submit, button(tr('Отмена'), modal.close, { secondary: true }));

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    submit.disabled = true;

    try {
      const percent = Number(input.value);
      const raised = isOwner ? await api.raiseRating(percent) : await api.raiseRatingOf(profile.nickname, percent);

      me.votes = raised.balance;
      page.completeness = raised.completeness;
      if (isOwner) me.rating = raised.rating;
      modal.close();
      showCompleteness(page, isOwner);
    } catch (e) {
      submit.disabled = false;
      showError(error, e);
    }
  });
  input.focus();
}

// showClubs is openvk's block "Группы": how many, "Все" and the names of
// the newest, as the user's "Кому видно мои группы" allows.
function showClubs(user) {
  showClubBlock('clubs', '/groups?u=' + encodeURIComponent(user.nickname), clubsCount, () => api.userClubs(user.nickname, '', CLUBS_SHOWN));
}

async function showClubBlock(id, all, count, list) {
  try {
    const page = await list();
    // Only the names: the checkmarks are for the club's own page.
    const links = page.clubs.map((club) => el('a', { href: pagePath({ ...club, club: true }), textContent: club.name }));

    $(id + '-count').textContent = count(page.total);
    $(id + '-all').href = all;
    $(id + '-list').replaceChildren(...links.flatMap((link, i) => (i === 0 ? [link] : [' • ', link])));
    $(id + '-block').hidden = page.total === 0;
  } catch {
    // Hidden by the user, or not to be had: the page works without it.
  }
}

// The newest gifts of the block "Подарки".
const GIFTS_SHOWN = 5;

// showGifts is openvk's block "Подарки" over the wall: how many, "Все"
// and the newest, as "Кому видны мои подарки" allows. Someone else's page
// offers "Отправить подарок" under the photo, as "Кто может отправлять
// мне подарки" allows; one's own gifts are bought in the shop.
async function showGifts(user, me, isOwner) {
  if (me && !isOwner && user.access.sendGift) {
    $('gift').href = pickPath(user);
    $('gift').hidden = false;
  }

  try {
    if (user.access.gifts) {
      const page = await api.gifts(user.nickname, '', GIFTS_SHOWN);

      $('gifts-block').replaceChildren(...giftsBlock(user, page));
      $('gifts-block').hidden = page.total === 0;
    }
  } catch {
    // Not to be had: the page works without it.
  }
}

// showTitles is the block "Титулы": the awards the administrators gave the
// page, each with its picture and, given more than once, how many times.
async function showTitles(user) {
  let titles;
  try {
    ({ titles } = await api.userTitles(user.nickname));
  } catch {
    return; // Not to be had: the page works without it.
  }
  if (!titles.length) return;

  $('titles-count').textContent = `${titles.length} ${plural(titles.length, ['титул', 'титула', 'титулов'])}`;
  $('titles-list').replaceChildren(...titleGroups(titles).map((g) => el('div', { className: 'title-side-row', title: tr('Получен {0}', formatDateTime(g.last)) },
    el('img', { className: 'title-badge', src: titleIconPath(g.icon), alt: '', width: 36, height: 36 }),
    el('div', { className: 'title-side-text' },
      el('b', { textContent: g.name }),
      g.count > 1 ? el('span', { className: 'title-times', textContent: ' ×' + g.count }) : null,
    ))));
  $('titles-block').hidden = false;
}

// showStories offers "Смотреть историю" under the photo while the user
// has live stories the viewer sees.
async function showStories(user) {
  let authors;
  try {
    ({ authors } = await api.userStories(user.nickname));
  } catch {
    return; // Not to be had: the page works without it.
  }

  if (authors.length === 0) return;

  $('stories').hidden = false;
  $('stories').addEventListener('click', () => openStories(authors, 0));
}

// renderHidden is openvk's page whose owner hides it from the viewer
// ("Кому в интернете видно мою страницу"): the name and why there is
// nothing else.
function renderHidden(profile) {
  const name = fullName(profile);

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(name, verifiedMark(profile));
  $('title-aside').textContent = '@' + profile.nickname;
  $('error').replaceChildren(el('b', { textContent: tr('Ошибка доступа') }), el('br'),
    tr('Настройки приватности этого пользователя не разрешают вам смотреть на его страницу.'));
  $('error').hidden = false;
}

// renderClosed is openvk's closed profile ("Закрытая страница") for those
// its owner is not friends with: the photo, the name and how to ask for
// more.
function renderClosed(profile) {
  const name = fullName(profile);
  const whose = { male: tr('его'), female: tr('её') }[profile.gender] ?? tr('его(её)');
  const whom = accusativeName(profile);

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(name, verifiedMark(profile));
  $('title-aside').textContent = '@' + profile.nickname;
  $('name').textContent = name;
  $('status').textContent = tr('Закрытая страница');
  $('status').classList.add('empty');
  $('limited').replaceChildren(
    el('b', { textContent: tr('{0} {1} доступ к своей странице.', name, limitedAccess(profile.gender)) }), el('br'),
    me
      ? tr('Страницу видят только друзья. Добавьте {0} в друзья, чтобы видеть {1} обновления.', whom, whose)
      : tr('Зарегистрируйтесь, чтобы получить доступ к {0} странице.', whose),
  );
  $('limited').hidden = false;
  $('profile-content').hidden = true;
  $('profile').hidden = false;
}

// renderLimited is openvk's page of someone blacklisted either way: the
// photo, the name and why there is nothing else. The data stays public
// (anonymous visitors see it), the page only stops inviting contact.
function renderLimited(profile) {
  const name = fullName(profile);

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(name, verifiedMark(profile));
  $('title-aside').textContent = '@' + profile.nickname;
  $('name').textContent = name;
  $('limited').textContent = profile.blockedMe
    ? tr('{0} {1} доступ к своей странице.', name, limitedAccess(profile.gender))
    : tr('{0} — в вашем чёрном списке.', name);
  $('limited').hidden = false;
  $('profile-content').hidden = true;
  $('profile').hidden = false;
}

// renderDeactivated is openvk's page that its owner deleted: the photo,
// the name, "Страница удалена" and nothing else. Its owner also gets the
// way back until the page is erased.
function renderDeactivated(profile, isOwner) {
  const name = fullName(profile);

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(name, verifiedMark(profile));
  $('title-aside').textContent = '@' + profile.nickname;
  $('name').textContent = name;
  $('status').textContent = tr('Страница удалена');
  $('status').classList.add('empty');
  $('profile-content').replaceChildren(el('p', { className: 'deleted-info' },
    tr('Страница пользователя удалена.'), el('br'), tr('Информация недоступна.')));
  $('profile').hidden = false;

  if (isOwner) {
    const restore = el('a', { textContent: tr('восстановить свою страницу'), onclick: reactivate });
    $('deactivated').replaceChildren(
      el('p', {}, tr('Ваша страница '), el('b', { textContent: tr('удалена') }), '.'),
      el('p', {}, tr('Если Вы захотите снова начать пользоваться сайтом, Вы можете '), restore,
        tr(' до {0}.', formatDateTime(me.purgeAt))),
    );
    $('deactivated').hidden = false;
  }
}

// renderBanned is a page the administrators of the site blocked: a plain
// window of the site with the reason and the term, as a notice of the
// 2000s; its owner reads it addressed to them, an administrator may lift
// the block.
function renderBanned(profile, isOwner) {
  const name = fullName(profile);
  const ban = profile.ban;

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(name, verifiedMark(profile));
  $('title-aside').textContent = '@' + profile.nickname;
  $('name').textContent = name;
  $('status').textContent = tr('Пользователь заблокирован');
  $('status').classList.add('empty');
  $('profile').hidden = false;

  const lead = isOwner
    ? el('p', {}, el('b', { textContent: name }), tr(', ваша страница заблокирована администрацией Freenet за нарушение правил сайта.'))
    : el('p', {}, tr('Страница пользователя '), el('b', { textContent: genitiveName(profile) }), tr(' заблокирована администрацией Freenet за нарушение правил сайта.'));
  const info = el('div', { className: 'game-window banned-info' },
    el('div', { className: 'game-window-title' },
      el('img', { src: '/static/icons/game/ban.svg', alt: '', width: 16, height: 16 }),
      el('span', { textContent: tr('Страница заблокирована') })),
    el('div', { className: 'banned-body' },
      lead,
      el('table', { className: 'ban-facts' },
        el('tr', {}, el('th', { textContent: tr('Причина:') }), el('td', {}, el('div', { className: 'ban-reason', textContent: ban.reason }))),
        el('tr', {}, el('th', { textContent: tr('Срок:') }), el('td', { textContent: banEnd(ban) })))),
  );

  if (isOwner) {
    info.lastChild.append(el('p', {}, el('a', { href: '/', textContent: tr('Выйти'), onclick: logout })));
  } else if (me?.admin) {
    const lift = el('a', { textContent: tr('Разблокировать пользователя') });
    lift.addEventListener('click', async () => {
      try {
        await api.unban(profile.nickname);
        location.reload();
      } catch (e) {
        showError($('error'), e);
      }
    });
    info.lastChild.append(el('p', {}, lift));
  }

  $('profile-content').replaceChildren(info);
}

async function reactivate() {
  try {
    await api.reactivate();
    location.reload();
  } catch (e) {
    showError($('error'), e);
  }
}

// The page is asked for with who is signed in, in one round trip.
const wanted = started(api.profile(nickname));
const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  // The public profile also brings the follow counters; the owner sees the
  // whole profile from /me. A deleted page sees others as a guest does.
  const profile = await (me?.deactivated || me?.ban ? api.publicProfile(nickname) : wanted);
  const isOwner = Boolean(me) && me.id === profile.id;

  if (profile.ban) {
    renderBanned(profile, isOwner);
    initPhoto(profile, false);
  } else if (profile.deactivated) {
    renderDeactivated(profile, isOwner);
    initPhoto(profile, false);
  } else if (profile.pageHidden && !profile.closed) {
    renderHidden(profile);
  } else if (profile.blockedMe || profile.blockedByMe) {
    renderLimited(profile);
    initPhoto(profile, false);
  } else if (profile.pageHidden) {
    renderClosed(profile);
    initPhoto(profile, false);
    if (me) {
      initFollow(profile, me, false);
      showWrite(profile, me);
    }
  } else {
    render(isOwner ? me : profile, isOwner, profile);
    showBackdrop(profile.backdrop);
    initPhoto(isOwner ? me : profile, isOwner);
    initFollow(profile, me, isOwner);
    // As openvk, the blocks the owner hides from the viewer are not there.
    if (profile.access.clubs) showClubs(profile);
    showGifts(profile, me, isOwner);
    showTitles(profile);
    showMusic(profile, me, isOwner);
    if (me) showStories(profile);
    // "Гости": the owner learns of the visit, unless it leaves no trace.
    if (me && !isOwner) api.visitPage(profile.nickname).catch(() => {});
    showCompleteness(isOwner ? me : profile, isOwner);
    showRaise(profile, isOwner);

    // As on openvk: write to anyone but yourself.
    if (me && !isOwner) showWrite(profile, me);
  }

  // openvk's orange label above the name with the words the
  // administrators wrote: on every page that shows the name, but not of a
  // blocked one.
  if (profile.alert && !profile.ban && (!profile.pageHidden || profile.closed)) {
    $('user-alert').textContent = profile.alert;
    $('user-alert').hidden = false;
  }

  // As on openvk, also on the page of whoever blacklisted the viewer.
  if (me && !isOwner && !profile.deactivated && !profile.ban && (!profile.pageHidden || profile.closed)) {
    initBlacklistLink(profile, () => location.reload());
    initIgnoreLink($('ignore'), {
      kind: 'user',
      ignored: profile.ignoredByMe,
      change: (ignore) => (ignore ? api.ignoreUser(profile.nickname) : api.unignoreUser(profile.nickname)),
      error: $('follow-error'),
    });
  }
} catch (e) {
  $('title').textContent = e.status === 404 || e.status === 400 ? tr('Страница не найдена') : tr('Ошибка');
  document.title = $('title').textContent + ' | Freenet';
  showError($('error'), e.status === 400 ? tr('Страница не найдена.') : e);
}
