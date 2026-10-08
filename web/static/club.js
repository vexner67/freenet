// The page of a community at /club/<address or id>, as openvk's: the name,
// "Информация" and the wall; on the left the photo, "Вступить в группу" or
// "Выйти из группы", "Редактировать группу" for its admins, a few members
// and its creator.

import { api, currentUser } from './api.js';
import { membersCount } from './labels.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';
import { personCell } from './people.js';
import { initWall } from './wall.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length));
const MEMBERS_SHOWN = 6;

const manages = (club) => club.role === 'admin' || club.role === 'owner';

function render(club, me) {
  document.title = `${club.name} | Freenet`;
  $('title').textContent = club.name;
  $('name').textContent = club.name;

  // The address of the page is its address, once it has one.
  const path = '/club/' + encodeURIComponent(clubRef(club));
  if (decodeURIComponent(location.pathname) !== decodeURIComponent(path)) history.replaceState(null, '', path);

  $('info').replaceChildren(...[
    ['Название:', club.name],
    ['Описание:', club.description],
  ].filter(([, value]) => value).map(([title, value]) => el('tr', {},
    el('th', { textContent: title }),
    el('td', { className: 'pre-wrap', textContent: value }),
  )));

  $('edit').hidden = !manages(club);
  $('edit').href = path + '/edit';

  renderOwner(club.owner);
  renderJoin(club, me);
  $('club').hidden = false;
}

function renderOwner(owner) {
  $('owner-block').hidden = !owner;
  if (!owner) return;

  $('owner').replaceChildren(el('div', { className: 'people-grid' }, personCell(owner)));
}

// renderJoin is the one button openvk has: "Вступить в группу", or "Выйти
// из группы" for a member; the owner cannot leave and has neither.
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

// showMembers is the block "Участники": how many, "Все" and a few faces.
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
  await initWall({ ...club, club: true }, me, { moderates: manages(club), speaksForClub: manages(club) });
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
  showError($('error'), e);
}
