// "Мои Сообщения", as on openvk: dialogs, the most recently written first.
// A row shows who, when, and the last message, grey while unread. New
// messages bring their dialog to the top live (events.js).

import { api, currentUser } from './api.js';
import { onEvent } from './events.js';
import { formatPostDate, snippet } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showError, hideError } from './layout.js';

const PREVIEW = 120;

let me;
let cursor = '';
// Peer ID → { dialog, node }, to move and redraw a dialog live.
const shown = new Map();

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

// show draws a dialog in place of its old row, or at the top.
function show(d, atTop) {
  const old = shown.get(d.peer.id);
  const node = dialogNode(d);
  shown.set(d.peer.id, { dialog: d, node });

  if (old && !atTop) {
    old.node.replaceWith(node);
    return node;
  }

  old?.node.remove();
  if (atTop) $('dialogs').prepend(node);
  $('empty').hidden = true;

  return node;
}

function listenToEvents() {
  onEvent('message', (e) => {
    show({ peer: e.peer, lastMessage: e.message, unread: !e.message.out }, true);
  });

  // Read on another page (byMe), or by the peer: the row stops being grey.
  onEvent('read', (e) => {
    const entry = shown.get(e.peer.id);
    if (!entry) return;

    const { dialog } = entry;
    const m = dialog.lastMessage;
    if (m.id > e.upTo) return;

    if (e.byMe && !m.out) dialog.unread = false;
    if (!e.byMe && m.out) m.read = true;
    show(dialog, false);
  });
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.dialogs(cursor);
    const fresh = page.dialogs.filter((d) => !shown.has(d.peer.id));

    $('dialogs').append(...fresh.map((d) => {
      const node = dialogNode(d);
      shown.set(d.peer.id, { dialog: d, node });
      return node;
    }));
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
  listenToEvents();
}
