// "Мои Группы", as on openvk: the tabs "Группы" and "Управляемые" (the
// clubs you run), a grey bar with the search by groups and "Создать
// группу", how many there are, and the clubs, the newest joined first,
// each with "Просмотр группы", "Добавить в левое меню" for those you run
// and "Выйти из группы".

import { api, currentUser } from './api.js';
import { clubsCount, myClubsCount } from './labels.js';
import { $, clubRef, el, pagePath, renderChrome, showPinnedClubs, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';

const managed = new URLSearchParams(location.search).get('act') === 'managed';
let cursor = '';

function clubNode(club) {
  const ref = clubRef(club);
  const node = clubRow(club,
    el('a', { className: 'profile-link', href: pagePath({ ...club, club: true }), textContent: 'Просмотр группы' }),
    (club.role === 'admin' || club.role === 'owner') && pinLink(club, ref),
    club.role !== 'owner' && leaveLink(club, ref, () => node),
  );

  return node;
}

// pinLink is "Добавить в левое меню" or "Удалить из левого меню".
function pinLink(club, ref) {
  const link = el('a', { className: 'profile-link' });
  const render = () => { link.textContent = club.pinned ? 'Удалить из левого меню' : 'Добавить в левое меню'; };

  link.addEventListener('click', async () => {
    hideError($('error'));

    try {
      if (club.pinned) await api.unpinClub(ref);
      else await api.pinClub(ref);
      club.pinned = !club.pinned;
      render();
      showPinnedClubs($('menu'));
    } catch (e) {
      showError($('error'), e);
    }
  });
  render();

  return link;
}

// leaveLink is "Выйти из группы"; the club then goes from the list.
function leaveLink(club, ref, node) {
  return el('a', {
    className: 'profile-link',
    textContent: 'Выйти из группы',
    onclick: async () => {
      hideError($('error'));

      try {
        await api.leaveClub(ref);
        node().replaceWith(el('div', { className: 'post-deleted', textContent: `Вы вышли из группы «${club.name}».` }));
        if (club.pinned) showPinnedClubs($('menu'));
      } catch (e) {
        showError($('error'), e);
      }
    },
  });
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.myClubs(cursor, managed);

    if (!cursor) $('summary').textContent = managed ? clubsCount(page.total) : myClubsCount(page.total);
    $('clubs').append(...page.clubs.map(clubNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
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
  $(managed ? 'tab-managed' : 'tab-all').classList.add('active');
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
