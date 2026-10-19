// "Редактировать группу", as on openvk: the name, the description, the
// address of the page, the site, who may write on the wall and what the
// page shows of its admins ("Список админов") and whether it is closed.
// Only the admins get here; the club service checks it again. The owner
// may delete the club too ("Вы также можете удалить группу"), which
// openvk does not let.

import { api, currentUser } from './api.js';
import { adminsDisplays, clubTypes } from './labels.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/edit'.length));

function fill(club) {
  const path = '/club/' + encodeURIComponent(clubRef(club));

  const heading = tr('Редактировать группу');

  document.title = `${heading} | Freenet`;
  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ` » ${heading}`);
  $('tab-main').href = path + '/edit';
  $('tab-backdrop').href = path + '/backdrop';
  $('tab-members').href = path + '/members';
  $('name').value = club.name;
  $('description').value = club.description;
  $('address').value = club.address;
  $('website').value = club.website;
  $('wall').value = club.wall;
  $('type').replaceChildren(...clubTypes.map(([value, text]) => el('option', { value, textContent: text })));
  $('type').value = club.closed ? 'closed' : 'open';
  $('admins-display').replaceChildren(...adminsDisplays.map(([value, text]) => el('label', {},
    el('input', { type: 'radio', name: 'admins-display', value, checked: value === club.adminsDisplay }), text)));
  $('open-topics').checked = club.openTopics;
  $('topics-above-wall').checked = club.topicsAboveWall;
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
      website: $('website').value.trim(),
      wall: $('wall').value,
      adminsDisplay: document.querySelector('input[name="admins-display"]:checked')?.value ?? 'creator',
      openTopics: $('open-topics').checked,
      topicsAboveWall: $('topics-above-wall').checked,
      closed: $('type').value === 'closed',
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

// confirmDelete asks before the club goes with its wall, topics, albums
// and photos: nothing brings it back.
function confirmDelete(club) {
  const modal = openModal(tr('Удалить группу'));
  modal.body.textContent = tr('Вы уверены, что хотите удалить группу «{0}»? Вместе с ней удалятся её участники, стена, обсуждения, альбомы и фотографии. Это нельзя отменить.', club.name);
  modal.actions.append(
    button(tr('Удалить'), async () => {
      try {
        await api.deleteClub(clubRef(club));
        location.assign('/groups');
      } catch (e) {
        modal.close();
        showError($('error'), e);
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
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
      if (club.role === 'owner') {
        $('delete-club').hidden = false;
        $('delete').addEventListener('click', () => confirmDelete(club));
      }
      $('tabs').hidden = false;
      $('form').hidden = false;
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? tr('Группа не найдена') : tr('Ошибка');
    showError($('error'), e);
    $('error').hidden = false;
    $('form').hidden = false;
  }
}
