// "Мои Группы", as on openvk: the tabs "Группы" and "Управляемые" (the
// clubs you run), a grey bar with the search by groups and "Создать
// группу", how many there are, and the clubs, the newest joined first,
// each with "Просмотр группы", "Добавить в левое меню" for those you run
// and "Выйти из группы". With ?u=<nickname> it is someone else's list, as
// "Все" of the block "Группы" on their page leads: "Имя » Группы", how
// many and the clubs, as their "Кому видно мои группы" allows. At /events
// it is "Мои Встречи": the same page for the events you come to, with
// "Создать встречу".

import { api, currentUser } from './api.js';
import { clubsCount, eventsCount, myClubsCount, myEventsCount } from './labels.js';
import { $, clubRef, el, fullName, pagePath, renderChrome, showPinnedClubs, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';

const params = new URLSearchParams(location.search);
const managed = params.get('act') === 'managed';
const other = params.get('u');
const events = location.pathname === '/events';
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

// load shows one more page: list gives it, node draws a club, summary
// says how many there are.
async function load(list, node, summary) {
  hideError($('error'));

  try {
    const page = await list(cursor);

    if (!cursor) $('summary').textContent = summary(page.total);
    $('clubs').append(...page.clubs.map((club) => node(club)));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    showError($('error'), e);
  }
}

// showMine is "Мои Группы" or, at /events, "Мои Встречи".
async function showMine() {
  if (events) {
    document.title = 'Мои Встречи | Freenet';
    $('title').textContent = 'Мои Встречи';
    $('tab-all').textContent = 'Встречи';
    $('tab-all').href = '/events';
    $('tab-managed').href = '/events?act=managed';
    $('create').textContent = 'Создать встречу';
    $('create').href = '/events/create';
    $('search').placeholder = 'Поиск встреч';
  }

  $(managed ? 'tab-managed' : 'tab-all').classList.add('active');
  $('tabs').hidden = false;
  $('gray-bar').hidden = false;

  const list = events ? api.myEvents : api.myClubs;
  const summary = events ? (managed ? eventsCount : myEventsCount) : (managed ? clubsCount : myClubsCount);
  const more = () => load((c) => list(c, managed), clubNode, summary);
  $('more').addEventListener('click', more);
  await more();
}

// showTheirs is someone else's list: only "Просмотр группы" by each club.
async function showTheirs(nickname) {
  try {
    const user = await api.profile(nickname);
    const name = fullName(user);

    document.title = `Группы: ${name} | Freenet`;
    $('title').replaceChildren(el('a', { href: '/' + user.nickname, textContent: name }), ' » Группы');
  } catch (e) {
    $('title').textContent = 'Группы';
    showError($('error'), e);
    return;
  }

  const view = (club) => clubRow(club,
    el('a', { className: 'profile-link', href: pagePath({ ...club, club: true }), textContent: 'Просмотр группы' }));
  const more = () => load((c) => api.userClubs(nickname, c), view, clubsCount);
  $('more').addEventListener('click', more);
  await more();
}

const me = await currentUser().catch(() => null);
if (other && other !== me?.nickname) {
  renderChrome(me);
  await showTheirs(other);
} else if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  await showMine();
}
