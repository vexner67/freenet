// The wall of a person (/wall/<nickname>) or a community
// (/club/<ref>/wall) on a page of its own, as openvk's /wall<id>: "Анна
// Петрова » Стена" with how many posts, the tabs "Все записи", "Записи
// Анны" (of a community, "Записи сообщества"), "Чужие записи", "Архив
// записей" for whoever owns the wall and "Поиск"; the form to write is on
// the first tab only.

import { api, currentUser } from './api.js';
import { genitiveName } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError } from './layout.js';
import { initWall, wallPath } from './wall.js';

const clubMatch = location.pathname.match(/^\/club\/([^/]+)\/wall$/);
const params = new URLSearchParams(location.search);
const type = ['owner', 'others', 'archive', 'search'].includes(params.get('type')) ? params.get('type') : '';
const query = type === 'search' ? (params.get('q') ?? '').trim() : '';

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
  const tabs = { '': 'tab-all', owner: 'tab-owner', others: 'tab-others', archive: 'tab-archive', search: 'tab-search' };

  // As openvk: the archive is its owner's only.
  $('tab-archive').hidden = !moderates;
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

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { owner, options } = await loadOwner();
  const name = owner.club ? owner.name : fullName(owner);

  document.title = `Стена | ${name} | Freenet`;
  $('title').replaceChildren(el('a', { href: pagePath(owner), textContent: name }), ' » Стена');
  const moderates = options.moderates ?? me?.id === owner.id;

  if (type === 'archive' && !moderates) throw new Error('permission denied');

  renderTabs(owner, moderates);
  $('wall').hidden = false;

  // "Поиск" searches all posts of the wall, once there is something to
  // search for.
  if (type !== 'search' || query) await initWall(owner, me, { ...options, type: type === 'search' ? '' : type, query });
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Стена';
  showError($('error'), e);
}
