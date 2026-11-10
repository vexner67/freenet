import { api, currentUser, started } from './api.js';
import { $, el, hideError, renderChrome, showError } from './layout.js';
import { roomRow } from './room-list.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const club = params.get('club') ?? '';
const owner = params.get('u') ?? '';
let cursor = '';
let shown = [];
const read = () => club ? api.clubRooms(club, cursor) : owner ? api.userRooms(owner, cursor) : api.rooms(cursor);
let first = started(read());
const me = await currentUser().catch(() => null);
renderChrome(me);

async function load(reset = false) {
  if (reset) cursor = '';
  try {
    const page = await (first ?? read());
    first = null;
    shown = reset ? page.rooms : [...shown, ...page.rooms.filter((room) => !shown.some((old) => old.id === room.id))];
    cursor = page.nextCursor;
    $('rooms-list').replaceChildren(...shown.map((room) => roomRow(room)));
    $('rooms-empty').hidden = shown.length > 0;
    $('more').hidden = !cursor;
  } catch (e) { first = null; showError($('error'), e); }
}

let manager = !club;
if (club && me) {
  try { const result = await api.club(club); manager = ['owner', 'admin'].includes(result.role); }
  catch (e) { showError($('error'), e); }
}
$('create-room').hidden = !me?.profileCompleted || !manager;
$('create-room').addEventListener('click', () => {
  $('create-room-form').hidden = !$('create-room-form').hidden;
  if (!$('create-room-form').hidden) $('room-title').focus();
});
if (club) {
  $('room-access').append(el('option', { value: 'club', textContent: tr('участники сообщества') }));
  $('room-access').value = 'club';
  $('room-permanent').checked = true;
}
$('create-room-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError($('error'));
  $('create-submit').disabled = true;
  try {
    const result = await api.createRoom({ title: $('room-title').value, kind: $('room-kind').value,
      access: $('room-access').value, control: $('room-control').value, permanent: $('room-permanent').checked, ...(club ? { clubId: club } : {}) });
    location.assign(`/room/${encodeURIComponent(result.room.id)}`);
  } catch (e) { showError($('error'), e); $('create-submit').disabled = false; }
});
$('more').addEventListener('click', () => load());
if (!me && !owner && !club) {
  $('rooms-empty').textContent = tr('Чтобы увидеть комнаты, войдите на сайт.');
  $('rooms-empty').hidden = false;
  first.catch(() => {});
} else {
  await load();
  if (me?.nickname && !owner && !club) {
    try {
      const page = await api.userRooms(me.nickname);
      $('permanent-rooms').replaceChildren(...page.rooms.map((room) => roomRow(room)));
      $('permanent-block').hidden = page.rooms.length === 0;
      if (shown.length === 0) {
        const friends = { users: [] };
        let next = '';
        do {
          const page = await api.friends(me.nickname, 'all', next, 50);
          friends.users.push(...page.users);
          next = page.nextCursor;
        } while (next);
        const pages = await Promise.allSettled(friends.users.map((friend) => api.userRooms(friend.nickname)));
        const rooms = pages.flatMap((page) => page.status === 'fulfilled' ? page.value.rooms : []);
        $('friends-rooms').replaceChildren(...rooms.map((room) => roomRow(room)));
        $('friends-block').hidden = rooms.length === 0;
      }
    } catch (e) { showError($('error'), e); }
  }
}
const timer = setInterval(() => { if (!document.hidden && shown.length <= 20 && (me || owner || club)) load(true); }, 15_000);
window.addEventListener('pagehide', () => clearInterval(timer), { once: true });
