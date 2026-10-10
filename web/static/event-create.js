// "Создать встречу", as on openvk: a name, a description, when it starts
// and, with "Указать дату окончания", when it ends; the new event opens
// at once, owned by you.

import { api, currentUser } from './api.js';
import { localTime } from './labels.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';

async function submit(event) {
  event.preventDefault();
  hideError($('error'));
  $('submit').disabled = true;

  try {
    const club = await api.createEvent({
      name: $('name').value,
      description: $('description').value,
      startsAt: localTime($('start-date').value, $('start-time').value),
      endsAt: $('has-end').checked ? localTime($('end-date').value, $('end-time').value) : '',
    });
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

  if (me.profileCompleted) {
    $('has-end').addEventListener('change', () => { $('end').hidden = !$('has-end').checked; });
    $('form').addEventListener('submit', submit);
  } else {
    $('form').replaceChildren(el('p', {}, 'Чтобы создать встречу, ', el('a', { href: '/edit', textContent: 'заполните свою страницу' }), '.'));
  }
}
