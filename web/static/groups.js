// "Мои Группы", as on openvk: the tabs "Группы" and "Управляемые" (the
// clubs you run), a grey bar with the search by groups and "Создать
// группу", how many there are, and the clubs, the newest joined first,
// each with "Просмотр группы", "Добавить в левое меню" for those you run
// and "Выйти из группы". With ?u=<nickname> it is someone else's list, as
// "Все" of the block "Группы" on their page leads: "Имя » Группы", how
// many and the clubs, as their "Кому видно мои группы" allows, each with
// "Вступить в группу" or "Выйти из группы" for you. At /events
// it is "Мои Встречи": the same page for the events you come to, with
// "Создать встречу" and the links named for an event ("Просмотр встречи",
// "Я приду", "Я не приду"); the events that are over stand apart, under
// "Прошедшие".

import { api, currentUser } from './api.js';
import { clubsCount, eventsCount, myClubsCount, myEventsCount, pastEventsCount } from './labels.js';
import { $, clubRef, el, fullName, pagePath, renderChrome, showPinnedClubs, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';

const params = new URLSearchParams(location.search);
const other = params.get('u');
const events = location.pathname === '/events';
const managed = params.get('act') === 'managed';
// The events that are over: "Прошедшие".
const past = events && params.get('act') === 'past';
let cursor = '';

// viewLink is "Просмотр группы", of an event "Просмотр встречи".
function viewLink(club) {
  return el('a', { className: 'profile-link', href: pagePath({ ...club, club: true }), textContent: events ? 'Просмотр встречи' : 'Просмотр группы' });
}

// The links of a membership, as the buttons of the club's page name them.
function joinText(club) {
  if (!events) return 'Вступить в группу';

  return new Date(club.startsAt) <= Date.now() ? 'Добавить в список' : 'Я приду';
}

const leaveText = () => (events ? 'Я не приду' : 'Выйти из группы');

function clubNode(club) {
  const ref = clubRef(club);
  const node = clubRow(club,
    viewLink(club),
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

// leaveLink is "Выйти из группы", of an event "Я не приду"; the club then
// goes from the list.
function leaveLink(club, ref, node) {
  return el('a', {
    className: 'profile-link',
    textContent: leaveText(),
    onclick: async () => {
      hideError($('error'));

      try {
        await api.leaveClub(ref);
        node().replaceWith(el('div', { className: 'post-deleted', textContent: events ? `Вы не придёте на встречу «${club.name}».` : `Вы вышли из группы «${club.name}».` }));
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
    $('tab-past').hidden = false;
    $('create').textContent = 'Создать встречу';
    $('create').href = '/events/create';
    $('search').placeholder = 'Поиск встреч';
  }

  $(managed ? 'tab-managed' : past ? 'tab-past' : 'tab-all').classList.add('active');
  $('tabs').hidden = false;
  $('gray-bar').hidden = false;

  // "Встречи" are those ahead, "Управляемые" all you run.
  const list = events ? (c) => api.myEvents(c, managed, managed ? '' : past ? 'past' : 'upcoming') : (c) => api.myClubs(c, managed);
  const summary = events ? (managed ? eventsCount : past ? pastEventsCount : myEventsCount) : (managed ? clubsCount : myClubsCount);
  const more = () => load(list, clubNode, summary);
  $('more').addEventListener('click', more);
  await more();
}

// membershipLink joins a club of someone else's list or leaves it, in
// place: club.role says which. The owner cannot leave and has none.
function membershipLink(club) {
  const ref = clubRef(club);
  const link = el('a', { className: 'profile-link' });
  const render = () => { link.textContent = club.role ? leaveText() : joinText(club); };

  link.addEventListener('click', async () => {
    hideError($('error'));

    try {
      const changed = await (club.role ? api.leaveClub(ref) : api.joinClub(ref));
      club.role = changed.role;
      render();
    } catch (e) {
      showError($('error'), e);
    }
  });
  render();

  return link;
}

// showTheirs is someone else's list: "Просмотр группы" by each club and,
// for a viewer with a page, "Вступить в группу" or "Выйти из группы".
// Their events are those ahead and, under "Прошедшие", those over.
async function showTheirs(nickname, me) {
  const what = events ? 'Встречи' : 'Группы';

  try {
    const user = await api.profile(nickname);
    const name = fullName(user);

    document.title = `${what}: ${name} | Freenet`;
    $('title').replaceChildren(el('a', { href: '/' + user.nickname, textContent: name }), ' » ' + what);
  } catch (e) {
    $('title').textContent = what;
    showError($('error'), e);
    return;
  }

  if (events) {
    const path = '/events?u=' + encodeURIComponent(nickname);

    $('tab-all').textContent = 'Встречи';
    $('tab-all').href = path;
    $('tab-past').href = path + '&act=past';
    $('tab-past').hidden = false;
    $('tab-managed').hidden = true;
    $(past ? 'tab-past' : 'tab-all').classList.add('active');
    $('tabs').hidden = false;
  }

  const view = (club) => clubRow(club,
    viewLink(club),
    me?.profileCompleted && club.role !== 'owner' && membershipLink(club));
  const list = events ? (c) => api.userEvents(nickname, past ? 'past' : 'upcoming', c) : (c) => api.userClubs(nickname, c);
  const more = () => load(list, view, events ? (past ? pastEventsCount : eventsCount) : clubsCount);
  $('more').addEventListener('click', more);
  await more();
}

const me = await currentUser().catch(() => null);
if (other && other !== me?.nickname) {
  renderChrome(me);
  await showTheirs(other, me);
} else if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  await showMine();
}
