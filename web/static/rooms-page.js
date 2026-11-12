// "Комнаты", as "Мои Группы" of openvk: the tabs "Комнаты" (the rooms one
// may enter now) and "Управляемые" (one's own), a grey bar with the search
// by rooms and "Создать комнату", how many there are, and the rooms, each
// with "Войти в комнату" and, for its owner, "Настройки" and "Закрыть
// комнату". With ?club=<ref> it is the rooms of a community, "Группа »
// Комнаты", and with ?u=<nickname> those of a person.

import { api, currentUser, started } from './api.js';
import { $, el, fullName, hideError, renderChrome, showError } from './layout.js';
import { roomListRow } from './room-list.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const club = params.get('club') ?? '';
const owner = params.get('u') ?? '';
const managed = !club && !owner && params.get('act') === 'managed';
let me = null;
let cursor = '';
let shown = [];
const read = () => {
  if (club) return api.clubRooms(club, cursor);
  if (owner) return api.userRooms(owner, cursor);
  if (managed) return mine();
  return api.rooms(cursor);
};

// mine are one's own rooms: the permanent ones (three at most) and the
// temporary ones open now, which close by themselves when left empty.
async function mine() {
  const [permanent, live] = await Promise.all([api.userRooms(me.nickname), api.rooms()]);
  const rooms = [...permanent.rooms, ...live.rooms.filter((room) => room.ownerId === me.id && !room.permanent && !room.clubId)];
  return { rooms, nextCursor: '' };
}
// The live rooms are asked for with who is signed in, in one round trip.
let first = managed ? null : started(read());
me = await currentUser().catch(() => null);
renderChrome(me);

// closeLink is "Закрыть комнату", asked once more: everyone inside leaves.
function closeLink(room) {
  return el('button', {
    className: 'profile-link', type: 'button', textContent: tr('Закрыть комнату'),
    onclick: async (event) => {
      const link = event.currentTarget;
      if (!window.confirm(tr('Закрыть комнату для всех участников?'))) return;
      hideError($('error'));
      link.disabled = true;
      try {
        await api.closeRoom(room.id);
        shown = shown.filter((old) => old.id !== room.id);
        draw();
      } catch (e) { showError($('error'), e); link.disabled = false; }
    },
  });
}

function roomNode(room) {
  const path = `/room/${encodeURIComponent(room.id)}`;
  const mine = room.ownerId === me?.id;
  return roomListRow(room,
    el('a', { className: 'profile-link', href: path, textContent: room.members.some((m) => m.user.id === me?.id) ? tr('Вернуться в комнату') : tr('Войти в комнату') }),
    mine && el('a', { className: 'profile-link', href: `${path}/edit`, textContent: tr('Настройки') }),
    mine && closeLink(room));
}

// applySearch leaves the rooms shown whose rows have the words typed.
function applySearch() {
  const words = $('search').value.trim().toLowerCase();
  const rows = [...$('rooms-list').querySelectorAll('.list-row')];
  for (const row of rows) row.hidden = !row.textContent.toLowerCase().includes(words);
  $('rooms-none').hidden = !words || rows.some((row) => !row.hidden);
}

function draw() {
  $('summary').textContent = tr('Комнат: {0}', `${shown.length}${cursor ? '+' : ''}`);
  $('rooms-list').replaceChildren(...shown.map(roomNode));
  $('rooms-list').hidden = shown.length === 0;
  $('rooms-empty').hidden = shown.length > 0;
  $('more').hidden = !cursor;
  applySearch();
}

async function load(reset = false) {
  if (reset) cursor = '';
  try {
    const page = await (first ?? read());
    first = null;
    shown = reset ? page.rooms : [...shown, ...page.rooms.filter((room) => !shown.some((old) => old.id === room.id))];
    cursor = page.nextCursor;
    draw();
  } catch (e) { first = null; showError($('error'), e); }
}

// heading names whose rooms these are: "Группа » Комнаты", "Имя » Комнаты".
async function heading() {
  const what = tr('Комнаты');
  if (!club && !owner) return true;
  try {
    const result = club ? await api.club(club) : await api.profile(owner);
    const name = club ? result.name : fullName(result);
    const path = club ? `/club/${encodeURIComponent(club)}` : `/${encodeURIComponent(owner)}`;
    document.title = `${what}: ${name} | Freenet`;
    $('title').replaceChildren(el('a', { href: path, textContent: name }), ` » ${what}`);
    return club ? ['owner', 'admin'].includes(result.role) : owner === me?.nickname;
  } catch (e) { showError($('error'), e); return false; }
}

if (!me && !owner && !club) {
  first?.catch(() => {});
  $('rooms-empty').textContent = tr('Чтобы увидеть комнаты, войдите на сайт.');
  $('rooms-empty').hidden = false;
} else if (managed && !me) {
  location.replace('/');
} else {
  const manager = await heading();
  if (!club && !owner) {
    $(managed ? 'tab-managed' : 'tab-all').classList.add('active');
    $('tabs').hidden = false;
  }
  $('gray-bar').hidden = false;
  $('create').hidden = !me?.profileCompleted || !manager;
  if (club) $('create').href = `/rooms/create?club=${encodeURIComponent(club)}`;
  $('rooms-empty').textContent = managed ? tr('У вас пока нет комнат. Создайте комнату и позовите друзей.')
    : club || owner ? tr('Здесь пока нет комнат.') : tr('Сейчас нет живых комнат. Создайте комнату и позовите друзей.');
  $('search').addEventListener('input', applySearch);
  $('more').addEventListener('click', () => load());
  await load();
  const timer = setInterval(() => { if (!document.hidden && shown.length <= 20) load(true); }, 15_000);
  window.addEventListener('pagehide', () => clearInterval(timer), { once: true });
}
