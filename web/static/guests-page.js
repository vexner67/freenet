// "Гости" (/guests): who came to the page of the signed-in person in the
// last 30 days, the last visit first. The guests that came after the last
// look here are new; a look here makes them seen, so the counter of the
// menu goes. A subscriber of Freenet Prime with "Невидимка" leaves no trace.

import { api, currentUser, session, started } from './api.js';
import { guestVisit, guestsCount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { personRow } from './people.js';
import { tr } from './i18n.js';

const first = session.get() ? started(api.guests()) : null;

let cursor = '';
let fresh = 0;

function guestRow(guest) {
  const row = personRow(guest.user,
    el('div', { className: 'guest-visit' },
      guest.new && el('b', { className: 'guest-new', textContent: tr('Новый гость') }),
      el('span', { className: 'hint', textContent: guestVisit(guest) })));
  if (guest.new) row.classList.add('guest-row-new');

  return row;
}

function show(page) {
  fresh += page.guests.filter((g) => g.new).length;
  $('list').append(...page.guests.map(guestRow));
  cursor = page.nextCursor;
  $('more').hidden = !cursor;

  const count = $('list').children.length;
  $('empty').hidden = count > 0;
  $('summary').hidden = count === 0;
  $('summary').textContent = fresh > 0
    ? tr('Показано {0}, из них новых: {1}', guestsCount(count), fresh)
    : tr('Показано {0}', guestsCount(count));
}

async function more() {
  hideError($('error'));
  $('more').hidden = true;
  try {
    show(await api.guests(cursor));
  } catch (e) {
    $('more').hidden = !cursor;
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  $('guest').hidden = false;
} else {
  try {
    show(await first);
    $('guests').hidden = false;
    // The look made the guests seen.
    const counter = document.getElementById('guests-count');
    if (counter) counter.hidden = true;
    $('more').addEventListener('click', more);
  } catch (e) {
    showError($('error'), e);
  }
}
