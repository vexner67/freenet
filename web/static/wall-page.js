// The wall of a person (/wall/<nickname>) or a community
// (/club/<ref>/wall) on a page of its own, as openvk's /wall<id>: "Анна
// Петрова » Стена" with how many posts, the tabs "Все записи", "Записи
// Анны" (of a community, "Записи сообщества") and "Чужие записи"; the form
// to write is on the first tab only.

import { api, currentUser } from './api.js';
import { genitiveName } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError } from './layout.js';
import { initWall, wallPath } from './wall.js';

const clubMatch = location.pathname.match(/^\/club\/([^/]+)\/wall$/);
const type = ['owner', 'others'].includes(new URLSearchParams(location.search).get('type'))
  ? new URLSearchParams(location.search).get('type')
  : '';

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

function renderTabs(owner) {
  const path = wallPath(owner);
  const tabs = { '': 'tab-all', owner: 'tab-owner', others: 'tab-others' };

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
  renderTabs(owner);
  $('wall').hidden = false;

  await initWall(owner, me, { ...options, type });
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Стена';
  showError($('error'), e);
}
