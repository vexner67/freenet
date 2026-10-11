// "Мои Сообщения", as on openvk: dialogs, the most recently written first.
// A row shows who, when, and the last message, grey while unread. New
// messages bring their dialog to the top live (events.js); a deleted last
// message makes the list load afresh, since only the server knows what is
// last now. The form above searches the messages (?q=): of all dialogs, or
// of the one the dialog page sent here from (?peer=<nickname>); a found
// message opens its dialog where it was said (?msg=<id>). Chats of several
// people ("беседы") stand in the same list, and "Новая беседа" starts one.

import { api, currentUser } from './api.js';
import { onEvent } from './events.js';
import { openNewChat, chatPath } from './chats.js';
import { chatPreview, chatTitle, formatPostDate, foundParts, foundSnippet, foundText, messagePreview, snippet } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showError, hideError } from './layout.js';

const PREVIEW = 120;

let me;
let cursor = '';
// Peer ID, or "chat:<id>" → { dialog, node }, to move and redraw a row live.
const shown = new Map();

const keyOf = (d) => (d.chat ? 'chat:' + d.chat.id : d.peer.id);

// chatNode is a chat of several people as a row: its name, when it was
// last written to, and who said what.
function chatNode(d) {
  const m = d.chat.lastMessage;

  return el('a', { className: 'dialog', href: chatPath(d.chat.id) },
    el('img', { className: 'dialog-avatar', src: '/static/chat.svg', alt: '' }),
    el('div', { className: 'dialog-info' },
      el('b', { className: 'dialog-name', textContent: chatTitle(d.chat) }),
      el('div', { className: 'dialog-date', textContent: formatPostDate(m?.createdAt ?? d.chat.createdAt) }),
    ),
    el('div', { className: d.unread ? 'dialog-message unread' : 'dialog-message' },
      m && el('img', { src: avatarURL(m.author, 'small'), alt: '' }),
      el('div', { className: 'dialog-text', textContent: snippet(chatPreview(m), PREVIEW) }),
    ),
  );
}

function dialogNode(d) {
  if (d.chat) return chatNode(d);

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
      // A message without words says what it shows, as on VK.
      el('div', { className: 'dialog-text', textContent: snippet(messagePreview(m), PREVIEW) }),
    ),
  );
}

// show draws a dialog in place of its old row, or at the top.
function show(d, atTop) {
  const old = shown.get(keyOf(d));
  const node = dialogNode(d);
  shown.set(keyOf(d), { dialog: d, node });

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

  onEvent('delete', (e) => {
    if (shown.get(e.peer.id)?.dialog.lastMessage.id === e.id) reload();
  });

  // The last message was edited or taken back: the row says so.
  onEvent('edit', (e) => {
    const entry = shown.get(e.peer.id);
    if (entry?.dialog.lastMessage.id !== e.message.id) return;

    entry.dialog.lastMessage = { ...e.message, read: entry.dialog.lastMessage.read };
    show(entry.dialog, false);
  });

  // A chat written to comes to the top; one not shown yet, as a new one I
  // was added to, is known only to the server.
  onEvent('chat_message', (e) => {
    const entry = shown.get('chat:' + e.chat);
    if (!entry) {
      reload();
      return;
    }

    const { dialog } = entry;
    dialog.chat.lastMessage = e.message;
    if (e.message.action === 'title') dialog.chat.title = e.message.text;
    dialog.unread = !e.message.out;
    show(dialog, true);
  });

  onEvent('chat_edit', (e) => {
    const entry = shown.get('chat:' + e.chat);
    if (entry?.dialog.chat.lastMessage?.id !== e.message.id) return;

    entry.dialog.chat.lastMessage = e.message;
    show(entry.dialog, false);
  });

  onEvent('chat_read', (e) => {
    const entry = shown.get('chat:' + e.chat);
    if (!entry || (entry.dialog.chat.lastMessage?.id ?? '') > e.upTo) return;

    entry.dialog.unread = false;
    show(entry.dialog, false);
  });
}

// reload draws the list from its first page again, one reload after
// another: two at once would interleave their rows.
let reloading = Promise.resolve();

function reload() {
  reloading = reloading.then(() => {
    shown.clear();
    cursor = '';
    $('dialogs').replaceChildren();
    return loadMore();
  });
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.dialogs(cursor);
    const fresh = page.dialogs.filter((d) => !shown.has(keyOf(d)));

    $('dialogs').append(...fresh.map((d) => {
      const node = dialogNode(d);
      shown.set(keyOf(d), { dialog: d, node });
      return node;
    }));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('dialogs').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

const params = new URLSearchParams(location.search);
const query = (params.get('q') ?? '').trim();
const searchPeer = params.get('peer') ?? '';

// foundNode is a found message as a row of the list: who it was written
// to or by, when, and what it says; it leads to where it was said in the
// dialog, the words found marked there too (&q=).
function foundNode({ peer, message: m }) {
  const href = `/im/${encodeURIComponent(peer.nickname)}?msg=${encodeURIComponent(m.id)}&q=${encodeURIComponent(query)}`;
  const said = foundText(m, query);
  const parts = foundParts(foundSnippet(said.text, query, PREVIEW), query);

  return el('a', { className: 'dialog', href },
    el('img', { className: 'dialog-avatar', src: avatarURL(peer, 'small'), alt: '' }),
    el('div', { className: 'dialog-info' },
      el('b', { className: 'dialog-name', textContent: fullName(peer) }),
      el('div', { className: 'dialog-date', textContent: formatPostDate(m.createdAt) }),
    ),
    el('div', { className: 'dialog-message' },
      m.out && el('img', { src: avatarURL(me, 'small'), alt: '' }),
      el('div', { className: 'dialog-text' },
        said.forwarded && el('span', { className: 'dialog-forwarded', textContent: 'Пересланное сообщение: ' }),
        ...parts.map((p) => (p.found ? el('span', { className: 'found-words', textContent: p.text }) : p.text))),
    ),
  );
}

async function searchMore() {
  hideError($('error'));

  try {
    const page = await api.searchMessages(query, { peer: searchPeer, cursor });

    $('dialogs').append(...page.messages.map(foundNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').textContent = 'Ничего не найдено.';
    $('empty').hidden = $('dialogs').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

// showSearch fills the form in and names the tab of the results.
function showSearch() {
  $('search-text').value = query;
  if (searchPeer) {
    $('search-peer').disabled = false;
    $('search-peer').value = searchPeer;
    $('search-text').placeholder = 'Поиск по переписке';
  }

  if (query || searchPeer) {
    $('tab-all').classList.remove('active');
    $('tab-search').classList.add('active');
    $('tab-search').hidden = false;
  }
}

me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  showSearch();
  $('new-chat').addEventListener('click', () => openNewChat(me));

  if (query) {
    $('more').addEventListener('click', searchMore);
    await searchMore();
  } else if (searchPeer) {
    $('search-text').focus();
  } else {
    $('more').addEventListener('click', loadMore);
    await loadMore();
    listenToEvents();
  }
}
