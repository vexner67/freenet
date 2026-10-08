// "Мои Группы", as on openvk: the clubs you are in, the newest joined
// first, each with its photo, name and members; "Создать группу" above.

import { api, currentUser } from './api.js';
import { clubRoles, membersCount } from './labels.js';
import { $, avatarURL, el, pagePath, renderChrome, showError, hideError } from './layout.js';

let cursor = '';

function clubNode(club) {
  const author = { ...club, club: true };
  const path = pagePath(author);

  return el('div', { className: 'club-row' },
    el('a', { href: path }, el('img', { src: avatarURL(author, 'small'), alt: '' })),
    el('div', {},
      el('a', { className: 'post-name', href: path, textContent: club.name }),
      el('div', { className: 'hint', textContent: membersCount(club.members) }),
      club.role !== 'member' && el('div', { className: 'club-role', textContent: clubRoles[club.role] }),
    ),
  );
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.myClubs(cursor);

    $('clubs').append(...page.clubs.map(clubNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('clubs').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  document.title = 'Мои Группы | Freenet';
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
