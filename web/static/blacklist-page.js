// The blacklist page (/settings/blacklist), openvk's "Чёрный список" tab
// of the settings: who is in it, newest first, each with "Удалить из
// чёрного списка".

import { api, currentUser } from './api.js';
import { blacklistCount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { personRow } from './people.js';
import { tr } from './i18n.js';

let cursor = '';
let total = 0;

function updateCount() {
  $('count').textContent = blacklistCount(total);
}

function row(user) {
  const node = el('div', { className: 'blacklist-row' },
    personRow(user),
    el('a', {
      textContent: tr('Удалить из чёрного списка'),
      onclick: async () => {
        hideError($('error'));

        try {
          await api.unblock(user.nickname);
          node.remove();
          total--;
          updateCount();
        } catch (e) {
          showError($('error'), e);
        }
      },
    }),
  );

  return node;
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.blacklist(cursor);

    total = page.total;
    cursor = page.nextCursor;
    $('people').append(...page.users.map(row));
    $('more').hidden = !cursor;
    updateCount();
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
