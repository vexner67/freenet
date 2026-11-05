// The page of a community at /club/<address or id>, as openvk's: on the
// left "Информация", "Участники" and the wall; on the right the photo,
// "Вступить в группу" or "Выйти из группы", "Редактировать группу" for
// its admins, "Тип группы" and, as its settings say, its creator, all its
// admins or nobody.

import { api, currentUser, started } from './api.js';
import { albumsBlock } from './albums.js';
import { showBackdrop } from './backdrop.js';
import { adminsCount, clubTypeHints, requestsLink, formatPostDate, membersCount, suggestedByEveryone, suggestedByYou, topicsCount } from './labels.js';
import { $, avatarURL, clubRef, el, fullName, hideError, renderChrome, showError, verifiedMark } from './layout.js';
import { personCell } from './people.js';
import { initPhoto } from './photo.js';
import { boardPath, topicPath } from './topics.js';
import { initIgnoreLink } from './feedsettings.js';
import { initWall } from './wall.js';
import { tr } from './i18n.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length));
const MEMBERS_SHOWN = 6;
const ADMINS_SHOWN = 5;
const TOPICS_SHOWN = 3;
const ALBUMS_SHOWN = 3;

const manages = (club) => club.role === 'admin' || club.role === 'owner';
// admitted: a closed group ("Закрытая группа") shows its wall, members,
// topics and albums to its members alone.
const admitted = (club) => !club.closed || Boolean(club.role);

function render(club, me) {
  document.title = `${club.name} | Freenet`;
  $('title').replaceChildren(club.name, verifiedMark(club));

  // The address of the page is its address, once it has one.
  const path = '/club/' + encodeURIComponent(clubRef(club));
  if (decodeURIComponent(location.pathname) !== decodeURIComponent(path)) history.replaceState(null, '', path);

  $('info').replaceChildren(...[
    [tr('Название:'), el('b', { textContent: club.name })],
    [tr('Описание:'), club.description && el('span', { className: 'pre-wrap', textContent: club.description })],
    [tr('Сайт:'), club.website && el('a', { href: club.website, target: '_blank', rel: 'ugc noopener noreferrer', textContent: club.website })],
  ].filter(([, value]) => value).map(([title, value]) => el('tr', {},
    el('th', { textContent: title }),
    el('td', {}, value),
  )));

  $('edit').hidden = !manages(club);
  $('edit').href = path + '/edit';
  if (me) {
    initIgnoreLink($('ignore'), {
      kind: 'club',
      ignored: Boolean(club.ignoredByMe),
      change: (ignore) => (ignore ? api.ignoreClub(ref) : api.unignoreClub(ref)),
      error: $('join-error'),
    });
  }

  initPhoto(club, manages(club), {
    upload: (file) => api.uploadClubAvatar(ref, file),
    remove: () => api.deleteClubAvatar(ref),
    error: $('join-error'),
    club: true,
  });
  $('type-text').textContent = clubTypeHints[club.closed ? 'closed' : 'open'];
  $('wall-block').hidden = !admitted(club);
  showRequests(club);
  renderOwner(club.adminsDisplay === 'creator' ? club.owner : null, club.ownerTitle);
  if (club.adminsDisplay === 'all') showAdmins(club);
  renderJoin(club, me);
  $('club').hidden = false;
}

// showRequests is the link to "Заявки" for those who run a closed group
// while anyone waits.
function showRequests(club) {
  $('requests').hidden = !manages(club) || !club.requests;
  $('requests-link').textContent = requestsLink(club.requests ?? 0);
  $('requests-link').href = `/club/${encodeURIComponent(clubRef(club))}/members?requests=true`;
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
      verifiedMark(user),
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
      el('div', { className: 'hint', textContent: tr('Обновлено ') + formatPostDate(topic.updatedAt) }),
    ));
    if (page.total === 0) {
      items.push(el('p', { className: 'side-text' }, el('a', { href: boardPath(club) + '/create', textContent: tr('Создать') })));
    }

    const block = el('div', { className: 'topics-block' },
      el('div', { className: 'section-title', textContent: tr('Обсуждения') }),
      el('div', { className: 'subtitle' }, el('span', { textContent: topicsCount(page.total) }), el('a', { href: boardPath(club), textContent: tr('Все') })),
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
  // A closed group: a visitor asks to join and may take the request back.
  if (club.closed && !club.role) return { join: tr('Подать заявку'), leave: tr('Отменить заявку') };

  return { join: tr('Вступить в группу'), leave: tr('Выйти из группы') };
}

// renderJoin is the one link openvk has: "Вступить в группу", or "Выйти
// из группы" for a member; the owner cannot leave and has neither.
function renderJoin(club, me) {
  const button = $('join');
  const error = $('join-error');
  const labels = joinLabels(club);
  const shown = Boolean(me?.profileCompleted) && club.role !== 'owner';

  button.hidden = !shown;
  // inside: a member, or someone whose request waits.
  const inside = Boolean(club.role) || Boolean(club.requested);

  button.textContent = inside ? labels.leave : labels.join;

  async function change(act) {
    hideError(error);
    button.disabled = true;

    try {
      const was = admitted(club);

      Object.assign(club, await act());
      // Who came into or left a closed group sees another page.
      if (was !== admitted(club)) {
        location.reload();
        return;
      }
      renderJoin(club, me);
      if (admitted(club)) showMembers(club);
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  }

  button.onclick = () => change(() => (inside ? api.leaveClub(ref) : api.joinClub(ref)));
}

// showMembers is the block "Участники": how many, "Все" and a row of
// faces.
async function showMembers(club) {
  await showFaces('members', club.members, `/club/${encodeURIComponent(clubRef(club))}/members`);
}

// showFaces fills one block of people: how many, "Все" and the first of
// them; nobody, no block.
async function showFaces(id, count, all) {
  $(id + '-block').hidden = count === 0;
  if (count === 0) return;

  try {
    const page = await api.clubMembers(ref, { limit: MEMBERS_SHOWN });

    $(id + '-count').textContent = membersCount(count);
    $(id + '-all').href = all;
    $(id + '-grid').replaceChildren(...page.members.map((m) => personCell(m.user)));
  } catch {
    // The page works without it.
  }
}

// The page is asked for with who is signed in, in one round trip.
const wanted = started(api.club(ref));
const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const club = await wanted;

  render(club, me);
  showBackdrop(club.backdrop);
  if (admitted(club)) {
    showMembers(club);
    showSuggestions(club, me);
    showTopics(club);
    showAlbums(club);
    await initWall({ ...club, club: true }, me, { moderates: manages(club), speaksForClub: manages(club) });
  }
} catch (e) {
  $('title').textContent = e.status === 404 ? tr('Группа не найдена') : tr('Ошибка');
  showError($('error'), e);
}
