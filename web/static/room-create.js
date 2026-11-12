// "Создать комнату", as "Создать группу": the name, what the room is for,
// who may enter and run the screen, whether it stays. With ?club=<id> the
// room is the community's, open to its members first. The creator enters
// the new room at once: it opens with ?join=1.

import { api, currentUser } from './api.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const club = new URLSearchParams(location.search).get('club') ?? '';

async function submit(event) {
  event.preventDefault();
  hideError($('error'));
  $('submit').disabled = true;

  try {
    const result = await api.createRoom({
      title: $('name').value,
      kind: $('kind').value,
      access: $('access').value,
      control: $('control').value,
      permanent: $('permanent').checked,
      ...(club ? { clubId: club } : {}),
    });
    location.assign(`/room/${encodeURIComponent(result.room.id)}?join=1`);
  } catch (e) {
    showError($('error'), e);
    $('submit').disabled = false;
  }
}

// forClub names the community over the form and lets its members in.
async function forClub() {
  $('access').append(el('option', { value: 'club', textContent: tr('участники сообщества') }));
  $('access').value = 'club';
  $('permanent').checked = true;
  try {
    const result = await api.club(club);
    $('title').replaceChildren(el('a', { href: `/club/${encodeURIComponent(club)}`, textContent: result.name }), ` » ${tr('Создать комнату')}`);
  } catch (e) { showError($('error'), e); }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  document.title = tr('Создать комнату | Freenet');

  if (me.profileCompleted) {
    if (club) await forClub();
    $('form').addEventListener('submit', submit);
    $('name').focus();
  } else {
    $('form').replaceChildren(el('p', {}, tr('Чтобы создать комнату, '), el('a', { href: '/edit', textContent: tr('заполните свою страницу') }), '.'));
  }
}
