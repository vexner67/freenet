// "Мои Группы", as on openvk: the tabs "Группы" and "Управляемые" (the
// clubs you run), a grey bar with the search by groups and "Создать
// группу", how many there are, and the clubs, the newest joined first,
// each with "Просмотр группы", "Добавить в левое меню" for those you run
// and "Выйти из группы". With ?u=<nickname> it is someone else's list, as
// "Все" of the block "Группы" on their page leads: "Имя » Группы", how
// many and the clubs, as their "Кому видно мои группы" allows, each with
// "Вступить в группу" or "Выйти из группы" for you.

import { api, currentUser, started } from './api.js';
import { clubsCount, myClubsCount } from './labels.js';
import { $, clubRef, el, fullName, pagePath, renderChrome, showPinnedClubs, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const other = params.get('u');
const managed = params.get('act') === 'managed';
let cursor = '';

// viewLink is "Просмотр группы".
function viewLink(club) {
  return el('a', { className: 'profile-link', href: pagePath({ ...club, club: true }), textContent: tr('Просмотр группы') });
}

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
  const render = () => { link.textContent = club.pinned ? tr('Удалить из левого меню') : tr('Добавить в левое меню'); };

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
    textContent: tr('Выйти из группы'),
    onclick: async () => {
      hideError($('error'));

      try {
        await api.leaveClub(ref);
        node().replaceWith(el('div', { className: 'post-deleted', textContent: tr('Вы вышли из группы «{0}».', club.name) }));
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
    const page = await (first ?? list(cursor));
    first = null;

    if (!cursor) $('summary').textContent = summary(page.total);
    $('clubs').append(...page.clubs.map((club) => node(club)));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    showError($('error'), e);
  }
}

// showMine is "Мои Группы".
async function showMine() {
  $(managed ? 'tab-managed' : 'tab-all').classList.add('active');
  $('tabs').hidden = false;
  $('gray-bar').hidden = false;

  const list = (c) => api.myClubs(c, managed);
  const summary = managed ? clubsCount : myClubsCount;
  const more = () => load(list, clubNode, summary);
  $('more').addEventListener('click', more);
  await more();
}

// membershipLink joins a club of someone else's list or leaves it, in
// place: club.role says which. The owner cannot leave and has none.
function membershipLink(club) {
  const ref = clubRef(club);
  const link = el('a', { className: 'profile-link' });
  // A closed group takes a request ("Подать заявку"), which may be taken back.
  const inside = () => Boolean(club.role) || Boolean(club.requested);
  const render = () => {
    if (club.closed && !club.role) link.textContent = club.requested ? tr('Отменить заявку') : tr('Подать заявку');
    else link.textContent = club.role ? tr('Выйти из группы') : tr('Вступить в группу');
  };

  link.addEventListener('click', async () => {
    hideError($('error'));

    try {
      const changed = await (inside() ? api.leaveClub(ref) : api.joinClub(ref));
      club.role = changed.role;
      club.requested = changed.requested;
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
async function showTheirs(nickname, me) {
  const what = tr('Группы');

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

  const view = (club) => clubRow(club,
    viewLink(club),
    me?.profileCompleted && club.role !== 'owner' && membershipLink(club));
  const list = (c) => api.userClubs(nickname, c);
  const more = () => load(list, view, clubsCount);
  $('more').addEventListener('click', more);
  await more();
}

// Your own clubs are asked for with who is signed in, in one round trip.
let first = other ? null : started(api.myClubs('', managed));
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
