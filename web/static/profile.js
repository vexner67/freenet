import { api, currentUser } from './api.js';
import { genders, relationships, partnerPrefix, takesPartner, worldviews, politicalViews, label, clubsCount, onlineCount, presence, formatBirthday, formatDateTime, limitedAccess } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError } from './layout.js';
import { albumPath, albumsBlock, isKeptBySite } from './albums.js';
import { initBlacklistLink } from './blacklist.js';
import { initFollow } from './follow.js';
import { initPhoto } from './photo.js';
import { initWall } from './wall.js';
import { showBackdrop } from './backdrop.js';
import { personCell } from './people.js';

const nickname = decodeURIComponent(location.pathname.slice(1));
const CLUBS_SHOWN = 6;
const ONLINE_SHOWN = 6;
// openvk shows the first page of albums on a person's page.
const ALBUMS_SHOWN = 10;

// rows lists the table under the name, in openvk's order; private marks
// rows others cannot see (only the owner gets them at all, from /me).
function rows(user, privacy) {
  return [
    ['Местоимения:', label(genders, user.gender), false],
    ['Семейное положение:', relationship(user), privacy && !privacy.showRelationship],
    ['Дата регистрации:', user.registeredAt ? formatDateTime(user.registeredAt) : '', false],
    ['Родной город:', user.hometown && searchLink('hometown', user.hometown), false],
    ['Полит. взгляды:', label(politicalViews, user.politicalViews), privacy && !privacy.showPoliticalViews],
    ['Мировоззрение:', label(worldviews, user.worldview), privacy && !privacy.showWorldview],
    ['Дата рождения:', user.birthday ? birthdayText(user) : '', privacy && !privacy.showBirthday],
  ].filter(([, value]) => value);
}

// relationship is "Замужем за Петром Петровым" as openvk says it: the
// status, then whom it names, a link.
function relationship(user) {
  const status = label(relationships(user.gender), user.relationship);
  if (!status || !user.partner || !takesPartner(user.relationship)) return status;

  return el('span', {},
    status + ' ' + partnerPrefix(user.relationship, user.gender) + ' ',
    el('a', { href: pagePath(user.partner), textContent: fullName(user.partner) }));
}

// birthdayText is the date; the owner who shows only the day and the month
// sees the year too, and that the others do not.
function birthdayText(user) {
  const text = formatBirthday(user.birthday);
  return user.hideBirthYear ? text + ' (год видите только вы)' : text;
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
// their owner put them.
function renderAbout(user, isOwner) {
  const c = user.contacts;
  const i = user.interests;
  const extra = (place) => (user.fields ?? []).filter((f) => f.place === place).map((f) => [f.name + ':', f.text]);

  const contacts = [
    ['Электронная почта:', c.email && external('mailto:' + c.email, c.email)],
    ['Telegram:', c.telegram && external('https://t.me/' + c.telegram, '@' + c.telegram)],
    ['Личный сайт:', c.website && external(c.website, c.website)],
    ['Город:', c.city && searchLink('city', c.city)],
    ['Адрес:', c.address],
    ...extra('contacts'),
  ];
  const interests = [
    ['Интересы:', i.interests],
    ['Любимая музыка:', i.music],
    ['Любимые фильмы:', i.films && linkedList('film', i.films)],
    ['Любимые ТВ-шоу:', i.shows && linkedList('show', i.shows)],
    ['Любимые книги:', i.books && linkedList('book', i.books)],
    ['Любимые цитаты:', i.quotes],
    ['Любимые игры:', i.games],
    ...extra('interests'),
    ['О себе:', i.about],
  ];

  for (const [id, list] of [['contacts', contacts], ['interests', interests]]) {
    const filled = list.filter(([, value]) => value);
    $(id).replaceChildren(...filled.map(([title, value]) => el('tr', {},
      el('th', { textContent: title }),
      el('td', { className: 'about-text' }, value))));
    $(id + '-none').hidden = filled.length > 0;
    $(id + '-edit').hidden = !isOwner;
  }
}

// renderHeader is openvk's: the name, "(это вы)" to the owner, and on the
// right "Онлайн" or when the person was last online. The owner, with this
// page open, is online.
function renderHeader(profile, isOwner) {
  const name = fullName(profile);
  const seen = isOwner ? 'Онлайн' : presence(profile);

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(name, ...(isOwner ? [' ', el('b', { textContent: '(это вы)' })] : []));
  $('title-aside').replaceChildren(seen === 'Онлайн' ? el('b', { textContent: seen }) : seen || '@' + profile.nickname);
}

function render(user, isOwner, profile) {
  const fullName = [user.name, user.surname].filter(Boolean).join(' ');

  renderHeader(profile, isOwner);
  $('name').textContent = fullName;

  const status = $('status');
  if (user.status) {
    status.textContent = user.status;
  } else if (isOwner) {
    status.replaceChildren(el('a', { href: '/edit', textContent: 'изменить статус' }));
    status.classList.add('empty');
  }

  const info = rows(user, isOwner ? user.privacy : null);
  $('info').replaceChildren(...info.map(([title, value, isPrivate]) => el('tr', {},
    el('th', { textContent: title }),
    el('td', {}, value, isPrivate && el('span', { className: 'private-mark', textContent: ' · видно только вам' })),
  )));
  renderAbout(user, isOwner);

  // "Кому видно основную информацию моей страницы": openvk leaves the
  // names, the photo and the status.
  $('info').hidden = Boolean(user.infoHidden);
  $('about-block').hidden = Boolean(user.infoHidden);

  $('edit').hidden = !isOwner;
  $('profile').hidden = false;

  initWall(user, me);
}

// showAlbums is openvk's block "Альбомы", when there are any.
async function showAlbums(user) {
  try {
    const page = await api.albums({ nickname: user.nickname, limit: ALBUMS_SHOWN });

    $('albums-block').replaceChildren(...albumsBlock(page.owner, page, false));
    $('albums-block').hidden = page.total === 0;
    linkAvatar(page.albums.find(isKeptBySite));
  } catch {
    // Not to be had: the page works without it.
  }
}

// showOnline is openvk's "Друзья онлайн", to the owner only: how many of
// the people they follow are online now, "Все" and the first of them.
async function showOnline(user) {
  try {
    const page = await api.online(user.nickname, '', ONLINE_SHOWN);

    $('online-count').textContent = onlineCount(page.total);
    $('online-all').href = `/follows?u=${encodeURIComponent(user.nickname)}&type=online`;
    $('online-grid').replaceChildren(...page.users.map((u) => personCell(u)));
    $('online-block').hidden = page.total === 0;
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
async function showClubs(user) {
  try {
    const page = await api.userClubs(user.nickname, '', CLUBS_SHOWN);
    const links = page.clubs.map((club) => el('a', { href: pagePath({ ...club, club: true }), textContent: club.name }));

    $('clubs-count').textContent = clubsCount(page.total);
    $('clubs-all').href = '/groups?u=' + encodeURIComponent(user.nickname);
    $('clubs-list').replaceChildren(...links.flatMap((link, i) => (i === 0 ? [link] : [' • ', link])));
    $('clubs-block').hidden = page.total === 0;
  } catch {
    // Hidden by the user, or not to be had: the page works without it.
  }
}

// renderHidden is openvk's page whose owner hides it from the viewer
// ("Кому в интернете видно мою страницу"): the name and why there is
// nothing else.
function renderHidden(profile) {
  const name = fullName(profile);

  document.title = `${name} | Freenet`;
  $('title').textContent = name;
  $('title-aside').textContent = '@' + profile.nickname;
  $('error').replaceChildren(el('b', { textContent: 'Ошибка доступа' }), el('br'),
    'Настройки приватности этого пользователя не разрешают вам смотреть на его страницу.');
  $('error').hidden = false;
}

// renderLimited is openvk's page of someone blacklisted either way: the
// photo, the name and why there is nothing else. The data stays public
// (anonymous visitors see it), the page only stops inviting contact.
function renderLimited(profile) {
  const name = fullName(profile);

  document.title = `${name} | Freenet`;
  $('title').textContent = name;
  $('title-aside').textContent = '@' + profile.nickname;
  $('name').textContent = name;
  $('limited').textContent = profile.blockedMe
    ? `${name} ${limitedAccess(profile.gender)} доступ к своей странице.`
    : `${name} — в вашем чёрном списке.`;
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
  $('title').textContent = name;
  $('title-aside').textContent = '@' + profile.nickname;
  $('name').textContent = name;
  $('status').textContent = 'Страница удалена';
  $('status').classList.add('empty');
  $('profile-content').replaceChildren(el('p', { className: 'deleted-info' },
    'Страница пользователя удалена.', el('br'), 'Информация недоступна.'));
  $('profile').hidden = false;

  if (isOwner) {
    const restore = el('a', { textContent: 'восстановить свою страницу', onclick: reactivate });
    $('deactivated').replaceChildren(
      el('p', {}, 'Ваша страница ', el('b', { textContent: 'удалена' }), '.'),
      el('p', {}, 'Если Вы захотите снова начать пользоваться сайтом, Вы можете ', restore,
        ` до ${formatDateTime(me.purgeAt)}.`),
    );
    $('deactivated').hidden = false;
  }
}

async function reactivate() {
  try {
    await api.reactivate();
    location.reload();
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  // The public profile also brings the follow counters; the owner sees the
  // whole profile from /me.
  const profile = await (me?.deactivated ? api.publicProfile(nickname) : api.profile(nickname));
  const isOwner = Boolean(me) && me.id === profile.id;

  if (profile.deactivated) {
    renderDeactivated(profile, isOwner);
    initPhoto(profile, false);
  } else if (profile.pageHidden) {
    renderHidden(profile);
  } else if (profile.blockedMe || profile.blockedByMe) {
    renderLimited(profile);
    initPhoto(profile, false);
  } else {
    render(isOwner ? me : profile, isOwner, profile);
    showBackdrop(profile.backdrop);
    initPhoto(isOwner ? me : profile, isOwner);
    initFollow(profile, me, isOwner);
    // As openvk, the blocks the owner hides from the viewer are not there.
    if (profile.access.photos) showAlbums(profile);
    if (profile.access.clubs) showClubs(profile);
    if (isOwner) showOnline(profile);

    // As on openvk: write to anyone but yourself.
    if (me && !isOwner) {
      $('write').href = '/im/' + encodeURIComponent(profile.nickname);
      $('write').hidden = false;
    }
  }

  // As on openvk, also on the page of whoever blacklisted the viewer.
  if (me && !isOwner && !profile.deactivated && !profile.pageHidden) {
    initBlacklistLink(profile, () => location.reload());
  }
} catch (e) {
  $('title').textContent = e.status === 404 || e.status === 400 ? 'Страница не найдена' : 'Ошибка';
  document.title = $('title').textContent + ' | Freenet';
  showError($('error'), e.status === 400 ? 'Страница не найдена.' : e);
}
