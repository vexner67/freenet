// The page of a community at /club/<address or id>, as openvk's: on the
// left "Информация", "Участники" and the wall; on the right the photo,
// "Вступить в группу" or "Выйти из группы", "Редактировать группу" for
// its admins, "Тип группы" and, as its settings say, its creator, all its
// admins or nobody.

import { api, currentUser } from './api.js';
import { adminsCount, formatPostDate, membersCount, suggestedByEveryone, suggestedByYou, topicsCount } from './labels.js';
import { $, avatarURL, clubRef, el, fullName, renderChrome, showError, hideError } from './layout.js';
import { personCell } from './people.js';
import { initPhoto } from './photo.js';
import { boardPath, topicPath } from './topics.js';
import { initWall } from './wall.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length));
const MEMBERS_SHOWN = 6;
const ADMINS_SHOWN = 5;
const TOPICS_SHOWN = 3;

const manages = (club) => club.role === 'admin' || club.role === 'owner';

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
  ].filter(([, value]) => value).map(([title, value]) => el('tr', {},
    el('th', { textContent: title }),
    el('td', {}, value),
  )));

  $('edit').hidden = !manages(club);
  $('edit').href = path + '/edit';

  initPhoto(club, manages(club), {
    upload: (file, onWall) => api.uploadClubAvatar(ref, file, onWall),
    remove: () => api.deleteClubAvatar(ref),
    error: $('join-error'),
    club: true,
  });
  renderOwner(club.adminsDisplay === 'creator' ? club.owner : null);
  if (club.adminsDisplay === 'all') showAdmins(club);
  renderJoin(club, me);
  $('club').hidden = false;
}

// renderOwner is openvk's "Создатель": a small photo and the name.
function renderOwner(owner) {
  $('owner-block').hidden = !owner;
  if (owner) $('owner').replaceChildren(avatarItem(owner));
}

function avatarItem(user) {
  const path = '/' + user.nickname;
  return el('div', { className: 'avatar-item' },
    el('a', { href: path }, el('img', { src: avatarURL(user, 'small'), alt: '' })),
    el('a', { href: path, textContent: fullName(user) }),
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
    $('admins').replaceChildren(...admins.map((m) => avatarItem(m.user)));
    $('admins-block').hidden = admins.length === 0;
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

// renderJoin is the one button openvk has: "Вступить в группу", or a grey
// "Выйти из группы" for a member; the owner cannot leave and has neither.
function renderJoin(club, me) {
  const button = $('join');
  const error = $('join-error');

  button.hidden = !me?.profileCompleted || club.role === 'owner';
  button.textContent = club.role ? 'Выйти из группы' : 'Вступить в группу';
  button.classList.toggle('secondary', Boolean(club.role));

  button.onclick = async () => {
    hideError(error);
    button.disabled = true;

    try {
      const changed = club.role ? await api.leaveClub(ref) : await api.joinClub(ref);
      Object.assign(club, changed);
      renderJoin(club, me);
      showMembers(club);
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  };
}

// showMembers is the block "Участники": how many, "Все" and a row of faces.
async function showMembers(club) {
  try {
    const page = await api.clubMembers(ref, { limit: MEMBERS_SHOWN });

    $('members-count').textContent = membersCount(club.members);
    $('members-all').href = `/club/${encodeURIComponent(clubRef(club))}/members`;
    $('members-grid').replaceChildren(...page.members.map((m) => personCell(m.user)));
    $('members-block').hidden = club.members === 0;
  } catch {
    // The page works without it.
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const club = await api.club(ref);

  render(club, me);
  showMembers(club);
  showSuggestions(club, me);
  showTopics(club);
  await initWall({ ...club, club: true }, me, { moderates: manages(club), speaksForClub: manages(club) });
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
  showError($('error'), e);
}
