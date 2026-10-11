// The page of a community at /club/<address or id>, as openvk's: on the
// left "Информация", "Участники" and the wall; on the right the photo,
// "Вступить в группу" or "Выйти из группы", "Редактировать группу" for
// its admins, "Тип группы" and, as its settings say, its creator, all its
// admins or nobody. An event ("встреча") is the same page with when it
// starts and ends, "Я приду" and "Возможно приду", those who come and
// those who might, and its "Организатор".

import { api, currentUser } from './api.js';
import { albumsBlock } from './albums.js';
import { showBackdrop } from './backdrop.js';
import { adminsCount, formatDateTime, formatPostDate, membersCount, suggestedByEveryone, suggestedByYou, topicsCount } from './labels.js';
import { $, avatarURL, clubRef, el, fullName, renderChrome, showError, hideError } from './layout.js';
import { personCell } from './people.js';
import { initPhoto } from './photo.js';
import { boardPath, topicPath } from './topics.js';
import { initIgnoreLink } from './feedsettings.js';
import { initWall } from './wall.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length));
const MEMBERS_SHOWN = 6;
const ADMINS_SHOWN = 5;
const TOPICS_SHOWN = 3;
const ALBUMS_SHOWN = 3;

const manages = (club) => club.role === 'admin' || club.role === 'owner';
const isEvent = (club) => club.kind === 'event';
// started: openvk takes an event that has begun for over.
const started = (club) => new Date(club.startsAt) <= new Date();

function render(club, me) {
  document.title = `${club.name} | Freenet`;
  $('title').textContent = club.name;

  // The address of the page is its address, once it has one.
  const path = '/club/' + encodeURIComponent(clubRef(club));
  if (decodeURIComponent(location.pathname) !== decodeURIComponent(path)) history.replaceState(null, '', path);

  $('info').replaceChildren(...[
    ['Название:', el('b', { textContent: club.name })],
    ['Описание:', club.description && el('span', { className: 'pre-wrap', textContent: club.description })],
    ['Сайт:', club.website && el('a', { href: club.website, target: '_blank', rel: 'ugc noopener noreferrer', textContent: club.website })],
    ['Начало:', club.startsAt && formatDateTime(club.startsAt)],
    ['Окончание:', club.endsAt && formatDateTime(club.endsAt)],
  ].filter(([, value]) => value).map(([title, value]) => el('tr', {},
    el('th', { textContent: title }),
    el('td', {}, value),
  )));

  $('edit').hidden = !manages(club);
  $('edit').href = path + '/edit';
  if (me) {
    initIgnoreLink($('ignore'), {
      kind: isEvent(club) ? 'event' : 'club',
      ignored: Boolean(club.ignoredByMe),
      change: (ignore) => (ignore ? api.ignoreClub(ref) : api.unignoreClub(ref)),
      error: $('join-error'),
    });
  }

  initPhoto(club, manages(club), {
    upload: (file, onWall) => api.uploadClubAvatar(ref, file, onWall),
    remove: () => api.deleteClubAvatar(ref),
    error: $('join-error'),
    club: true,
  });
  // An event always names who holds it, as openvk does.
  if (isEvent(club)) {
    $('type-title').textContent = 'Тип встречи';
    $('type-text').textContent = 'Это открытая встреча. В ней может поучаствовать любой желающий.';
    $('owner-title').textContent = 'Организатор';
    $('edit').textContent = 'Редактировать встречу';
  }
  renderOwner(club.adminsDisplay === 'creator' || isEvent(club) ? club.owner : null, club.ownerTitle);
  if (club.adminsDisplay === 'all') showAdmins(club);
  renderJoin(club, me);
  $('club').hidden = false;
}

// renderOwner is openvk's "Создатель": a small photo, the name and what
// the owner does in the club.
function renderOwner(owner, title) {
  $('owner-block').hidden = !owner;
  if (owner) $('owner').replaceChildren(avatarItem(owner, title));
}

// avatarItem is a small photo and the name with, under it, what the
// person does in the club, as openvk shows the comment on an admin.
function avatarItem(user, title) {
  const path = '/' + user.nickname;
  return el('div', { className: 'avatar-item' },
    el('a', { href: path }, el('img', { src: avatarURL(user, 'small'), alt: '' })),
    el('div', {},
      el('a', { href: path, textContent: fullName(user) }),
      title && el('div', { className: 'hint', textContent: title }),
    ),
  );
}

// showAdmins is openvk's "Администраторы": how many, "Все" and the first
// of them, the owner first.
async function showAdmins(club) {
  try {
    const page = await api.clubMembers(ref, { limit: ADMINS_SHOWN, admins: true });
    const admins = page.members.toSorted((a, b) => (b.role === 'owner') - (a.role === 'owner'));

    $('admins-count').textContent = adminsCount(page.total);
    $('admins-all').href = `/club/${encodeURIComponent(clubRef(club))}/members?admins=true`;
    $('admins').replaceChildren(...admins.map((m) => avatarItem(m.user, m.title)));
    $('admins-block').hidden = admins.length === 0;
  } catch {
    // The page works without it.
  }
}

// showAlbums is openvk's block "Альбомы": there are albums, or the
// viewer runs the community and may add the first.
async function showAlbums(club) {
  try {
    const page = await api.albums({ club: ref, limit: ALBUMS_SHOWN });

    $('albums-block').replaceChildren(...albumsBlock({ ...club, club: true }, page, manages(club)));
    $('albums-block').hidden = page.total === 0 && !manages(club);
  } catch {
    // The page works without it.
  }
}

// showSuggestions is the link above the wall of openvk: "N предложенных
// записей" for the admins, "N предложенных вами записей" for whoever
// suggested some, none when there are none.
async function showSuggestions(club, me) {
  const admin = manages(club);
  if (!me || (!admin && club.wall !== 'suggest')) return;

  try {
    const page = await api.clubSuggestions(ref, { mine: !admin, limit: 1 });
    if (page.total === 0) return;

    $('suggestions-link').textContent = admin ? suggestedByEveryone(page.total) : suggestedByYou(page.total);
    $('suggestions-link').href = `/club/${encodeURIComponent(clubRef(club))}/suggested${admin ? '' : '?type=my'}`;
    $('suggestions').hidden = false;
  } catch {
    // The page works without it.
  }
}

// showTopics is openvk's block "Обсуждения": how many, "Все" and the
// most recently active topics with "Обновлено", or "Создать" when there
// are none; above the wall or in the right column, as the club chooses.
// Nobody sees it in a club without topics where they may start none.
async function showTopics(club) {
  try {
    const page = await api.clubTopics(ref, { limit: TOPICS_SHOWN });
    if (page.total === 0 && !club.canStartTopic) return;

    const items = page.topics.map((topic) => el('div', { className: 'topic-short' },
      el('b', {}, el('a', { href: topicPath(topic), textContent: topic.title })),
      el('div', { className: 'hint', textContent: 'Обновлено ' + formatPostDate(topic.updatedAt) }),
    ));
    if (page.total === 0) {
      items.push(el('p', { className: 'side-text' }, el('a', { href: boardPath(club) + '/create', textContent: 'Создать' })));
    }

    const block = el('div', { className: 'topics-block' },
      el('div', { className: 'section-title', textContent: 'Обсуждения' }),
      el('div', { className: 'subtitle' }, el('span', { textContent: topicsCount(page.total) }), el('a', { href: boardPath(club), textContent: 'Все' })),
      ...items,
    );

    if (club.topicsAboveWall) $('wall-title').before(block);
    else $('club-side').append(block);
  } catch {
    // The page works without it.
  }
}

// joinLabels are the buttons of openvk: what a visitor presses to join,
// and what a member presses to leave.
function joinLabels(club) {
  if (!isEvent(club)) return { join: 'Вступить в группу', leave: 'Выйти из группы' };

  return { join: started(club) ? 'Добавить в список' : 'Я приду', leave: 'Я не приду' };
}

// renderJoin is the one button openvk has: "Вступить в группу", or a grey
// "Выйти из группы" for a member; the owner cannot leave and has neither.
// An event that has not started has a second one, "Возможно приду"; who
// answered changes the answer with it, without leaving: "Я приду" for
// who only might come.
function renderJoin(club, me) {
  const button = $('join');
  const maybe = $('join-maybe');
  const error = $('join-error');
  const labels = joinLabels(club);
  const shown = Boolean(me?.profileCompleted) && club.role !== 'owner';

  button.hidden = !shown;
  button.textContent = club.role ? labels.leave : labels.join;
  button.classList.toggle('secondary', Boolean(club.role));
  maybe.hidden = !shown || !isEvent(club) || started(club);
  maybe.textContent = club.role && club.maybe ? 'Я приду' : 'Возможно приду';

  async function change(act) {
    hideError(error);
    button.disabled = true;
    maybe.disabled = true;

    try {
      Object.assign(club, await act());
      renderJoin(club, me);
      showMembers(club);
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
      maybe.disabled = false;
    }
  }

  button.onclick = () => change(() => (club.role ? api.leaveClub(ref) : api.joinClub(ref)));
  maybe.onclick = () => change(() => api.joinClub(ref, { maybe: !(club.role && club.maybe) }));
}

// showMembers is the block "Участники": how many, "Все" and a row of
// faces. An event has two: "Точно придут" and, if anyone, "Возможно
// придут".
async function showMembers(club) {
  const path = `/club/${encodeURIComponent(clubRef(club))}/members`;

  if (!isEvent(club)) {
    await showFaces('members', club.members, path, {});
    return;
  }

  $('members-title').textContent = 'Точно придут';
  await Promise.all([
    showFaces('members', club.members - club.maybes, path + '?going=sure', { going: 'sure' }),
    showFaces('maybe', club.maybes, path + '?going=maybe', { going: 'maybe' }),
  ]);
}

// showFaces fills one block of people: how many, "Все" and the first of
// them; nobody, no block.
async function showFaces(id, count, all, filter) {
  $(id + '-block').hidden = count === 0;
  if (count === 0) return;

  try {
    const page = await api.clubMembers(ref, { limit: MEMBERS_SHOWN, ...filter });

    $(id + '-count').textContent = membersCount(count);
    $(id + '-all').href = all;
    $(id + '-grid').replaceChildren(...page.members.map((m) => personCell(m.user)));
  } catch {
    // The page works without it.
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const club = await api.club(ref);

  render(club, me);
  showBackdrop(club.backdrop);
  showMembers(club);
  showSuggestions(club, me);
  showTopics(club);
  showAlbums(club);
  await initWall({ ...club, club: true }, me, { moderates: manages(club), speaksForClub: manages(club) });
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
  showError($('error'), e);
}
