// "Создать группу", as on openvk: a name and a description; the new club
// opens at once, owned by you.

import { api, currentUser } from './api.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

async function submit(event) {
  event.preventDefault();
  hideError($('error'));
  $('submit').disabled = true;

  try {
    const club = await api.createClub($('name').value, $('description').value);
    location.href = '/club/' + encodeURIComponent(clubRef(club));
  } catch (e) {
    showError($('error'), e);
    $('submit').disabled = false;
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  document.title = tr('Создать группу | Freenet');

  if (me.profileCompleted) {
    $('form').addEventListener('submit', submit);
  } else {
    $('form').replaceChildren(el('p', {}, tr('Чтобы создать группу, '), el('a', { href: '/edit', textContent: tr('заполните свою страницу') }), '.'));
  }
}
