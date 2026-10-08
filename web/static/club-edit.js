// "Редактировать группу", as on openvk: the name, the description, the
// address of the page and who may write on the wall. Only the admins get
// here; the club service checks it again.

import { api, currentUser } from './api.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/edit'.length));

function fill(club) {
  const path = '/club/' + encodeURIComponent(clubRef(club));

  document.title = 'Редактировать группу | Freenet';
  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ' » Редактировать группу');
  $('tab-main').href = path + '/edit';
  $('tab-members').href = path + '/members';
  $('name').value = club.name;
  $('description').value = club.description;
  $('address').value = club.address;
  $('wall').value = club.wall;
  preview();
}

// preview shows where the page will be: its address, or its ID without one.
function preview() {
  $('url-preview').textContent = `${location.host}/club/${$('address').value || 'ID'}`;
}

async function submit(event, club) {
  event.preventDefault();
  hideError($('error'));
  $('saved').hidden = true;
  $('submit').disabled = true;

  try {
    const saved = await api.updateClub(clubRef(club), {
      name: $('name').value,
      description: $('description').value,
      address: $('address').value.trim(),
      wall: $('wall').value,
    });

    Object.assign(club, saved);
    fill(club);
    history.replaceState(null, '', '/club/' + encodeURIComponent(clubRef(club)) + '/edit');
    $('saved').hidden = false;
  } catch (e) {
    showError($('error'), e);
  } finally {
    $('submit').disabled = false;
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const club = await api.club(ref);

    if (club.role !== 'admin' && club.role !== 'owner') {
      location.replace('/club/' + encodeURIComponent(clubRef(club)));
    } else {
      fill(club);
      $('address').addEventListener('input', preview);
      $('form').addEventListener('submit', (event) => submit(event, club));
      $('tabs').hidden = false;
      $('form').hidden = false;
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
    showError($('error'), e);
    $('error').hidden = false;
    $('form').hidden = false;
  }
}
