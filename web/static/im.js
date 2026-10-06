// "Мои Сообщения", as on openvk: dialogs, the most recently written first.
// A row shows who, when, and the last message, grey while unread.

import { api, currentUser } from './api.js';
import { formatPostDate, snippet } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showError, hideError } from './layout.js';

const PREVIEW = 120;

let me;
let cursor = '';

function dialogNode(d) {
  const m = d.lastMessage;
  // Unread either way, as on openvk: the peer's message I have not read,
  // or mine the peer has not read yet.
  const unread = m.out ? !m.read : d.unread;

  return el('a', { className: 'dialog', href: '/im/' + encodeURIComponent(d.peer.nickname) },
    el('img', { className: 'dialog-avatar', src: avatarURL(d.peer, 'small'), alt: '' }),
    el('div', { className: 'dialog-info' },
      el('b', { className: 'dialog-name', textContent: fullName(d.peer) }),
      el('div', { className: 'dialog-date', textContent: formatPostDate(m.createdAt) }),
    ),
    el('div', { className: unread ? 'dialog-message unread' : 'dialog-message' },
      m.out && el('img', { src: avatarURL(me, 'small'), alt: '' }),
      el('div', { className: 'dialog-text', textContent: snippet(m.text, PREVIEW) }),
    ),
  );
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.dialogs(cursor);

    $('dialogs').append(...page.dialogs.map(dialogNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('dialogs').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
