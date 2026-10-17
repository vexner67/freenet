// The wall of a person (/wall/<nickname>) or a community
// (/club/<ref>/wall) on a page of its own, as openvk's /wall<id>: "Анна
// Петрова » Стена" with how many posts, the tabs "Все записи", "Записи
// Анны" (of a community, "Записи сообщества"), "Чужие записи", "Архив
// записей" for whoever owns the wall and "Поиск"; the form to write is on
// the first tab only. VK's "Отложенные записи" (openvk has none), the
// posts that wait for their time, are a tab of the same page.

import { api, currentUser, started } from './api.js';
import { archiveYears, genitiveName } from './labels.js';
import { $, clubRef, el, fullName, hideError, pagePath, renderChrome, showError } from './layout.js';
import { button, openModal } from './modal.js';
import { showPostponedList } from './postponed.js';
import { initWall, wallPath } from './wall.js';

const clubMatch = location.pathname.match(/^\/club\/([^/]+)\/wall$/);
const params = new URLSearchParams(location.search);
const type = ['owner', 'others', 'archive', 'postponed', 'search'].includes(params.get('type')) ? params.get('type') : '';
const query = type === 'search' ? (params.get('q') ?? '').trim() : '';
// The year of the archive, as openvk's ?year=; none is "Все годы".
const year = type === 'archive' && /^[1-9]\d{3}$/.test(params.get('year') ?? '') ? Number(params.get('year')) : 0;

// loadOwner is the person or the community whose wall this is, as initWall
// takes it, with what the viewer may do there.
async function loadOwner() {
  if (clubMatch) {
    const club = await api.club(decodeURIComponent(clubMatch[1]));
    const manages = club.role === 'admin' || club.role === 'owner';
    return { owner: { ...club, club: true }, options: { moderates: manages, speaksForClub: manages } };
  }

  const owner = await api.profile(decodeURIComponent(location.pathname.slice('/wall/'.length)));
  return { owner, options: {} };
}

function renderTabs(owner, moderates) {
  const path = wallPath(owner);
  const tabs = { '': 'tab-all', owner: 'tab-owner', others: 'tab-others', archive: 'tab-archive', postponed: 'tab-postponed', search: 'tab-search' };

  // As openvk: the archive is its owner's only.
  $('tab-archive').hidden = !moderates;
  $('tab-postponed').hidden = !moderates;
  $('archive-banner').hidden = type !== 'archive';
  $('wall-search').hidden = type !== 'search';
  $('wall-search').action = path;
  $('wall-search-text').value = query;

  for (const [tab, id] of Object.entries(tabs)) {
    $(id).href = tab ? `${path}?type=${tab}` : path;
    $(id).classList.toggle('active', tab === type);
  }

  // openvk says "Записи Анна"; we decline the first name.
  $('tab-owner').textContent = owner.club
    ? 'Записи сообщества'
    : 'Записи ' + genitiveName({ name: owner.name, gender: owner.gender });
}

// archivePath is the archive of the wall for a year, 0 for all of them.
function archivePath(owner, y) {
  return `${wallPath(owner)}?type=archive` + (y ? `&year=${y}` : '');
}

// confirmArchive asks before a whole year, or the whole wall, is moved:
// openvk's confirm of "Действия". y is the year, 0 for every post.
function confirmArchive(owner, y, archived) {
  const modal = openModal(archived ? 'Архивирование записей' : 'Разархивирование записей');
  const error = el('div', { className: 'error', hidden: true });
  const question = !y
    ? 'Архивировать все записи на этой стене?'
    : archived
      ? `Архивировать все записи, опубликованные в ${y} году?`
      : `Разархивировать все записи, опубликованные в ${y} году?`;

  const yes = button(archived ? 'Архивировать' : 'Разархивировать', async () => {
    hideError(error);
    yes.disabled = true;

    try {
      if (owner.club) await api.archiveClubWall(clubRef(owner), y, archived);
      else await api.archiveWall(owner.nickname, y, archived);
      // As openvk: on to the archive of the year.
      location.href = archivePath(owner, y);
    } catch (e) {
      showError(error, e);
      yes.disabled = false;
    }
  });

  modal.body.append(error, el('div', { textContent: question }));
  modal.actions.append(yes, button('Отмена', modal.close, { secondary: true }));
}

// renderArchive shows openvk's bar of the archive: "Все годы", the years
// the wall was written in (those past the sixth under "Ещё") and
// "Действия" with a whole year, or the whole wall, to put away or return.
function renderArchive(owner, years) {
  const { shown, more, action } = archiveYears(years, year);
  const yearLink = (y) => el('a', { className: y === year ? 'archive-year current' : 'archive-year', href: archivePath(owner, y), textContent: String(y) });

  $('archive-years').replaceChildren(
    el('a', { className: year ? 'archive-all' : 'archive-all active', href: archivePath(owner, 0), textContent: 'Все годы' }),
    ...shown.map(yearLink),
    more.length
      ? el('details', { className: 'archive-more' },
        el('summary', { textContent: more.includes(year) ? `Ещё: ${year}` : 'Ещё' }),
        el('div', { className: 'archive-menu' }, ...more.map(yearLink)))
      : '',
  );

  const act = (text, y, archived) => el('button', {
    type: 'button',
    textContent: text,
    onclick: () => {
      $('archive-actions').open = false;
      confirmArchive(owner, y, archived);
    },
  });

  $('archive-actions-menu').replaceChildren(
    ...(action
      ? [
        act(`Архивировать все записи за ${action}`, action, true),
        act(`Разархивировать все записи за ${action}`, action, false),
        el('div', { className: 'archive-divider' }),
      ]
      : []),
    act('Архивировать все записи целиком', 0, true),
  );

  $('archive-manager').hidden = false;
}

// A click beside an open menu of the bar closes it.
document.addEventListener('click', (event) => {
  for (const menu of document.querySelectorAll('.archive-manager details[open]')) {
    if (!menu.contains(event.target)) menu.open = false;
  }
});

// The page is asked for with who is signed in, in one round trip.
const wanted = started(loadOwner());
const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { owner, options } = await wanted;
  const name = owner.club ? owner.name : fullName(owner);

  document.title = `Стена | ${name} | Freenet`;
  $('title').replaceChildren(el('a', { href: pagePath(owner), textContent: name }), ' » Стена');
  const moderates = options.moderates ?? me?.id === owner.id;

  if ((type === 'archive' || type === 'postponed') && !moderates) throw new Error('permission denied');

  renderTabs(owner, moderates);
  $('wall').hidden = false;

  // "Поиск" searches all posts of the wall, once there is something to
  // search for.
  if (type === 'postponed') {
    await showPostponedList(owner);
  } else if (type !== 'search' || query) {
    await initWall(owner, me, {
      ...options, type: type === 'search' ? '' : type, query, year, onYears: (years) => renderArchive(owner, years),
    });
  }
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Стена';
  showError($('error'), e);
}
