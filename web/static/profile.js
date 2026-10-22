import { api, currentUser, started } from './api.js';
import { relationships, partnerName, partnerPrefix, takesPartner, worldviews, politicalViews, label, clubsCount, completenessHints, presence, formatBirthday, formatDateTime, limitedAccess, closedFollows, banEnd, genitiveName } from './labels.js';
import { $, el, fullName, hideError, logout, pagePath, renderChrome, showError, verifiedMark } from './layout.js';
import { albumPath, albumsBlock, isAvatarAlbum } from './albums.js';
import { giftsBlock, pickPath } from './gifts.js';
import { initBlacklistLink } from './blacklist.js';
import { initIgnoreLink } from './feedsettings.js';
import { initFollow } from './follow.js';
import { initPhoto } from './photo.js';
import { initWall } from './wall.js';
import { showBackdrop } from './backdrop.js';
import { personCell } from './people.js';
import { openStories } from './stories.js';
import { tr } from './i18n.js';

const nickname = decodeURIComponent(location.pathname.slice(1));
const CLUBS_SHOWN = 6;
// openvk shows the first page of albums on a person's page.
const ALBUMS_SHOWN = 10;

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
  // Freenet Prime: the name is golden, and so is the frame of the photo.
  document.body.classList.toggle('prime-page', Boolean(profile.prime));
  $('title').replaceChildren(profile.prime ? el('span', { className: 'prime-name', textContent: name }) : name,
    verifiedMark(profile), ...(isOwner ? [' ', el('b', { textContent: tr('(это вы)') })] : []));
  $('title-aside').replaceChildren(isOwner || profile.online ? el('b', { textContent: seen }) : seen || '@' + profile.nickname);
}

function render(user, isOwner, profile) {
  const fullName = [user.name, user.surname].filter(Boolean).join(' ');

  renderHeader(profile, isOwner);
  $('name').textContent = fullName;

  showStatus(user.status, isOwner);

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

// showStatus is the status under the name; to the owner it is openvk's
// "[ изменить статус ]" when empty, and either opens the box to edit it.
function showStatus(text, isOwner) {
  const status = $('status');
  status.classList.toggle('empty', !text);
  status.classList.toggle('status-editable', isOwner);
  status.replaceChildren(text || (isOwner ? tr('[ изменить статус ]') : ''));
  status.onclick = isOwner ? () => editStatus(text) : null;
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

// showCompleteness is openvk's gauge of how full the page is, to whoever
// is signed in, and to the owner the hints of what to fill in and, as
// openvk, "Повысить рейтинг".
function showCompleteness(user, isOwner) {
  const percent = user.completeness ?? 0;
  if (!me || !percent) return;

  const bar = el('div');
  bar.style.width = Math.min(percent, 100) + '%';

  const hints = isOwner
    ? (user.unfilled ?? []).filter((part) => completenessHints[part]).map((part) => {
      const [href, text] = completenessHints[part];
      return el('a', { className: 'completeness-hint hint-' + part, href, textContent: text });
    })
    : [];

  $('completeness').replaceChildren(
    el('div', { className: 'completeness-gauge' + (percent >= 100 ? ' completeness-gauge-gold' : '') },
      bar, el('span', { textContent: percent + '%' })),
    ...(hints.length ? [el('div', { className: 'completeness-hints' }, ...hints)] : []),
    ...(isOwner ? [el('a', { className: 'completeness-raise', href: '/votes?act=rating', textContent: tr('Повысить рейтинг') })] : []),
  );
  $('completeness').hidden = false;
}

// showAlbums is openvk's block "Альбомы", when there are any.
async function showAlbums(user) {
  try {
    const page = await api.albums({ nickname: user.nickname, limit: ALBUMS_SHOWN });

    $('albums-block').replaceChildren(...albumsBlock(page.owner, page, false));
    $('albums-block').hidden = page.total === 0;
    linkAvatar(page.albums.find(isAvatarAlbum));
  } catch {
    // Not to be had: the page works without it.
  }
}

// linkAvatar makes the photo of the page lead to "Фотографии со страницы",
// as openvk opens the photo there.
function linkAvatar(album) {
  if (!album || album.photos === 0) return;

  // The link takes the photo's place first: made around it, it would
  // take the photo out of the page before.
  const avatar = $('avatar');
  const link = el('a', { href: albumPath(album) });
  avatar.before(link);
  link.append(avatar);
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

// showGifts is openvk's block "Подарки" over the wall: how many, "Отправить
// подарок", "Все" and the newest, as "Кому видны мои подарки" allows. A
// page without gifts to show offers "Отправить подарок" under the photo.
async function showGifts(user, me, isOwner) {
  let shown = false;

  try {
    if (user.access.gifts) {
      const page = await api.gifts(user.nickname, '', GIFTS_SHOWN);

      shown = page.total > 0;
      $('gifts-block').replaceChildren(...giftsBlock(user, page, Boolean(me), isOwner));
      $('gifts-block').hidden = !shown;
    }
  } catch {
    // Not to be had: the page works without it.
  }

  if (me && !isOwner && !shown) {
    $('gift').href = pickPath(user);
    $('gift').hidden = false;
  }
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
// its owner does not follow: the photo, the name and how to ask for more.
function renderClosed(profile) {
  const name = fullName(profile);
  const whose = { male: tr('его'), female: tr('её') }[profile.gender] ?? tr('его(её)');

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(name, verifiedMark(profile));
  $('title-aside').textContent = '@' + profile.nickname;
  $('name').textContent = name;
  $('status').textContent = tr('Закрытая страница');
  $('status').classList.add('empty');
  $('limited').replaceChildren(
    el('b', { textContent: tr('{0} {1} доступ к своей странице.', name, limitedAccess(profile.gender)) }), el('br'),
    me
      ? tr('Страницу видят те, на кого {0} {1}. Вы можете подписаться на {2} обновления.', name, closedFollows(profile.gender), whose)
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

// renderBanned is a page the administrators of the site blocked: others
// read openvk's "К сожалению, нам пришлось заблокировать страницу
// пользователя …" with the reason, its owner openvk's "вы были
// верискокнуты" with the way out; an administrator may lift the block.
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

  if (isOwner) {
    $('profile-content').replaceChildren(el('div', { className: 'banned-info' },
      el('p', {}, tr('Извините, '), el('b', { textContent: name }), tr(', но вы были верискокнуты.')),
      el('p', {}, tr('А причина этому проста: '), el('b', { textContent: ban.reason }), '.'),
      ban.until
        ? el('p', {}, tr('На этот раз нам пришлось заблокировать вас до '), el('b', { textContent: formatDateTime(ban.until) }), '.')
        : el('p', { textContent: tr('К сожалению, на этот раз нам пришлось заблокировать вас навсегда.') }),
      el('p', {}, tr('Вы всё ещё можете '), el('a', { href: '/', textContent: tr('выйти'), onclick: logout }), '.'),
    ));
    return;
  }

  const info = el('div', { className: 'banned-info' },
    el('p', {}, tr('К сожалению, нам пришлось заблокировать страницу пользователя '), el('b', { textContent: genitiveName(profile) }), '.'),
    el('p', {}, tr('Комментарий модератора: '), el('b', { textContent: ban.reason }), '.'),
    el('p', {}, tr('Пользователь заблокирован '), el('b', { textContent: banEnd(ban) }), '.'),
  );

  if (me?.admin) {
    const lift = el('a', { textContent: tr('Разблокировать пользователя') });
    lift.addEventListener('click', async () => {
      try {
        await api.unban(profile.nickname);
        location.reload();
      } catch (e) {
        showError($('error'), e);
      }
    });
    info.append(el('p', {}, lift));
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
      $('write').href = '/im/' + encodeURIComponent(profile.nickname);
      $('write').hidden = false;
    }
  } else {
    render(isOwner ? me : profile, isOwner, profile);
    showBackdrop(profile.backdrop);
    initPhoto(isOwner ? me : profile, isOwner);
    initFollow(profile, me, isOwner);
    // As openvk, the blocks the owner hides from the viewer are not there.
    if (profile.access.photos) showAlbums(profile);
    if (profile.access.clubs) showClubs(profile);
    showGifts(profile, me, isOwner);
    if (me) showStories(profile);
    showCompleteness(isOwner ? me : profile, isOwner);

    // As on openvk: write to anyone but yourself.
    if (me && !isOwner) {
      $('write').href = '/im/' + encodeURIComponent(profile.nickname);
      $('write').hidden = false;
    }
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
