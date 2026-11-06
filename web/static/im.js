// "Сообщения", as openvk's messenger: the tab "Диалоги", a grey bar with
// the search, "Избранное" and "Новая беседа", the dialogs, the most
// recently written first, and a grey bar under them with how many there
// are and "Только непрочитанные", which leaves the unread ones alone.
// A row shows who, when, and the last message, grey while unread. New
// messages bring their dialog to the top live (events.js); a deleted last
// message makes the list load afresh, since only the server knows what is
// last now. The form above searches the messages (?q=): of all dialogs, or
// of the one the dialog or the chat page sent here from (?peer=<nickname>,
// ?chat=<id>); a found message opens its dialog or chat where it was said
// (?msg=<id>). Chats of several people ("беседы") stand in the same list,
// and "Новая беседа" starts one. Messages of dialogs are sealed
// (e2ee.js): the page opens the last ones itself, and searches a dialog
// itself, page by page, the server searching chats only. While someone
// types in a dialog or a chat, its row says so instead of the last
// message, as on VK (typing.js).

import { api, currentUser, started } from './api.js';
import { openMessages } from './e2ee.js';
import { onEvent } from './events.js';
import { openNewChat, chatPath } from './chats.js';
import { chatMembers, chatPreview, chatTitle, formatPostDate, plural, primeNameClass, foundParts, foundSnippet, foundText, messagePreview, snippet } from './labels.js';
import { $, avatarURL, chatPhotoURL, el, fullName, renderChrome, savedPath, showError, hideError, smileyNodes, verifiedMark } from './layout.js';
import { fitHeight, lastTab } from './messenger.js';
import { typingLine } from './typing.js';
import { tr } from './i18n.js';

const PREVIEW = 120;

let me;
let cursor = '';
// Peer ID, or "chat:<id>" → { dialog, node }, to move and redraw a row live.
const shown = new Map();

const keyOf = (d) => (d.chat ? 'chat:' + d.chat.id : d.peer.id);

// Row key → typingLine of who types there, and what the row says of them
// meanwhile instead of the last message.
const typing = new Map();
const typingNow = new Map();

// typistsOf keeps who types in the row with key, redrawing its text.
function typistsOf(key) {
  if (!typing.has(key)) {
    typing.set(key, typingLine((text) => {
      if (text) typingNow.set(key, text);
      else typingNow.delete(key);

      const entry = shown.get(key);
      if (entry) show(entry.dialog, false);
    }));
  }

  return typing.get(key);
}

// textNode is the line of a row: who types, or else the last message.
function textNode(d, preview) {
  const now = typingNow.get(keyOf(d));

  if (now) return el('div', { className: 'crp-entry--message---text dialog-typing' }, el('span', { textContent: now }));

  return el('div', { className: 'crp-entry--message---text' }, el('span', {}, ...smileyNodes(snippet(preview, PREVIEW))));
}

// nameNode is the name of a row as on the pages: coloured as its owner
// dressed it with Prime, the checkmark after it.
const nameNode = (person) => [el('span', { className: primeNameClass(person), textContent: fullName(person) }), verifiedMark(person)];

// rowNode is a row as openvk's ConversationItem (components/common.js):
// the photo, the name with how many are in a chat and the date under it,
// then the last message with the photo of who wrote it (none when the
// peer did) — the grey box when it is mine and not read yet — and the
// whole row grey while I have not read it.
function rowNode({ href, unread, unseen, photo, name, excess, author, authorName, text }) {
  return el('a', { className: ['crp-entry', unread && 'unread', !author && 'crp-entry-replied-same'].filter(Boolean).join(' '), href },
    el('div', { className: 'crp-entry--main' },
      el('div', { className: 'crp-entry--image' }, el('img', { src: photo, alt: '' })),
      el('div', { className: 'crp-entry--info' },
        el('div', { className: 'crp-entry--info-wrap' },
          el('b', { className: 'crp-entry--name' }, ...name),
          el('div', { className: 'crp-entry--excess' }, ...excess.filter(Boolean).map((t) => el('span', { textContent: t })))))),
    el('div', { className: unseen ? 'crp-entry--message unread' : 'crp-entry--message' },
      author && el('div', { className: 'crp-entry--message---av' }, el('img', { src: avatarURL(author, 'small'), alt: '' })),
      el('div', { className: 'crp-entry--message---content' },
        authorName && el('div', { className: 'crp-entry--message---author' }, ...authorName),
        text)),
  );
}

// chatNode is a chat of several people as a row: its name, how many are
// in it, when it was last written to, and who said what.
function chatNode(d) {
  const m = d.chat.lastMessage;

  return rowNode({
    href: chatPath(d.chat.id),
    unread: d.unread,
    photo: chatPhotoURL(d.chat, 'small'),
    name: [chatTitle(d.chat)],
    excess: [chatMembers(d.chat.memberCount), formatPostDate(m?.createdAt ?? d.chat.createdAt)],
    author: m?.author,
    // What happened in the chat names who did it itself.
    authorName: m?.author && !m.action ? nameNode(m.author) : null,
    text: textNode(d, chatPreview(m)),
  });
}

function dialogNode(d) {
  if (d.chat) return chatNode(d);

  const m = d.lastMessage;

  return rowNode({
    href: '/im/' + encodeURIComponent(d.peer.nickname),
    unread: d.unread,
    // Mine the peer has not read yet, as on openvk.
    unseen: m.out && !m.read && d.peer.id !== me.id,
    photo: avatarURL(d.peer, 'small'),
    name: d.peer.id === me.id ? [tr('Избранное')] : nameNode(d.peer),
    excess: [formatPostDate(m.createdAt)],
    author: m.out ? me : null,
    // A message without words says what it shows, as on VK.
    text: textNode(d, messagePreview(m)),
  });
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
  countShown();

  return node;
}

function listenToEvents() {
  onEvent('typing', (e) => {
    const key = e.chat ? 'chat:' + e.chat : e.user.id;
    if (shown.has(key)) typistsOf(key).typing(e.user);
  });

  onEvent('message', async (e) => {
    if (!e.message.out) typing.get(e.peer.id)?.stopped(e.peer.id);
    await openMessages(me.id, [e.message]);
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
  onEvent('edit', async (e) => {
    const entry = shown.get(e.peer.id);
    if (entry?.dialog.lastMessage.id !== e.message.id) return;

    await openMessages(me.id, [e.message]);
    entry.dialog.lastMessage = { ...e.message, read: entry.dialog.lastMessage.read };
    show(entry.dialog, false);
  });

  // A chat written to comes to the top; one not shown yet, as a new one I
  // was added to, is known only to the server.
  onEvent('chat_message', (e) => {
    if (e.message.author) typing.get('chat:' + e.chat)?.stopped(e.message.author.id);

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
    const page = await (first ?? api.dialogs(cursor));
    first = null;
    const fresh = page.dialogs.filter((d) => !shown.has(keyOf(d)));

    await openMessages(me.id, fresh.filter((d) => !d.chat).map((d) => d.lastMessage));
    $('dialogs').append(...fresh.map((d) => {
      const node = dialogNode(d);
      shown.set(keyOf(d), { dialog: d, node });
      return node;
    }));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    countShown();
  } catch (e) {
    showError($('error'), e);
  }
}

// countShown says under the list how many dialogs there are, once all are
// loaded, and says "Тут ничего нет" over an empty list, or one with no
// unread dialog while only those are shown.
function countShown() {
  const rows = $('dialogs').children.length;
  const unreadOnly = $('dialogs').classList.contains('unread-only');
  const seen = unreadOnly ? $('dialogs').querySelectorAll('.crp-entry.unread').length : rows;

  $('bottom').hidden = rows === 0;
  $('count').textContent = cursor ? '' : `${rows} ${plural(rows, ['переписка', 'переписки', 'переписок'])}`;
  $('empty').hidden = seen > 0;
}

// The bar under the list shows the unread dialogs alone, or all again.
function toggleUnread() {
  const only = $('dialogs').classList.toggle('unread-only');

  $('only-unread').textContent = only ? tr('Все переписки') : tr('Только непрочитанные');
  countShown();
}

// loadOnScroll pages on as the list, which scrolls inside the window as
// openvk's crp-list, nears its end.
function loadOnScroll(more) {
  let loading = false;

  $('list').addEventListener('scroll', async () => {
    const list = $('list');
    if (loading || $('more').hidden || list.scrollHeight - list.scrollTop - list.clientHeight > 250) return;

    loading = true;
    await more();
    loading = false;
  });
}

const params = new URLSearchParams(location.search);
const query = (params.get('q') ?? '').trim();
const searchPeer = params.get('peer') ?? '';
const searchChat = params.get('chat') ?? '';

// foundNode is a found message as a row of the list: who it was written
// to or by, when, and what it says; it leads to where it was said in the
// dialog, the words found marked there too (&q=).
function foundNode({ peer, message, chat, chatMessage }) {
  // A message of a chat stands under the name of the chat, with who
  // wrote it.
  const m = message ?? chatMessage;
  const where = chat ? chatPath(chat.id) : '/im/' + encodeURIComponent(peer.nickname);
  const href = `${where}?msg=${encodeURIComponent(m.id)}&q=${encodeURIComponent(query)}`;
  const said = foundText(m, query);
  const parts = foundParts(foundSnippet(said.text, query, PREVIEW), query);

  return rowNode({
    href,
    photo: chat ? chatPhotoURL(chat, 'small') : avatarURL(peer, 'small'),
    name: chat ? [chatTitle(chat)] : nameNode(peer),
    excess: [formatPostDate(m.createdAt)],
    author: chat ? m.author : m.out ? me : null,
    authorName: chat ? nameNode(m.author) : null,
    text: el('div', { className: 'crp-entry--message---text' },
      el('span', {},
        said.forwarded && el('span', { className: 'dialog-forwarded', textContent: tr('Пересланное сообщение: ') }),
        ...parts.map((p) => (p.found ? el('span', { className: 'found-words', textContent: p.text }) : p.text)))),
  });
}

async function searchMore() {
  hideError($('error'));
  $('more').hidden = true;

  try {
    const page = searchPeer ? await searchDialog() : await api.searchMessages(query, { chat: searchChat, cursor });

    $('dialogs').append(...page.messages.map(foundNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').textContent = tr('Ничего не найдено.');
    $('empty').hidden = $('dialogs').children.length > 0;
  } catch (e) {
    showError($('error'), e);
    $('more').hidden = !cursor;
  }
}

// How many messages of a dialog one page of its search reads, and in
// how many requests at most.
const SEARCH_PAGE = 50;
const SEARCH_READS = 20;
const FOUND_PAGE = 20;

// searchDialog finds the messages of the dialog with searchPeer that say
// query, newest first, opening them page by page: only this browser can
// read them. One call reads until it found a page of them or read
// SEARCH_READS pages; "Показать ещё" reads on from where it stopped.
async function searchDialog() {
  const found = [];
  const needle = query.toLowerCase();
  const says = (m) => [m.text, ...(m.forwarded ?? []).map((f) => f.text)].some((t) => t?.toLowerCase().includes(needle));
  let read = cursor;

  for (let i = 0; i < SEARCH_READS && found.length < FOUND_PAGE; i++) {
    const page = await api.messages(searchPeer, { cursor: read, limit: SEARCH_PAGE });

    await openMessages(me.id, page.messages);
    found.push(...page.messages.filter((m) => !m.deleted && says(m)).reverse().map((message) => ({ peer: page.peer, message })));
    read = page.nextCursor;
    if (!read) break;
  }

  return { messages: found, nextCursor: read };
}

// showSearch fills the form in and names the tab of the results.
function showSearch() {
  $('search-text').value = query;
  if (searchPeer) {
    $('search-peer').disabled = false;
    $('search-peer').value = searchPeer;
    $('search-text').placeholder = tr('Поиск по переписке');
  }

  if (searchChat) {
    $('search-chat').disabled = false;
    $('search-chat').value = searchChat;
    $('search-text').placeholder = tr('Поиск по переписке');
  }

  // "Просмотр диалогов" leads back to the conversation viewed last, as on
  // openvk.
  const last = lastTab();
  if (last) {
    $('tab-last').href = last;
    $('tab-last').hidden = false;
  }

  if (query || searchPeer || searchChat) {
    $('tab-all').classList.remove('active');
    $('tab-search').classList.add('active');
    $('tab-search').hidden = false;
  }

  // The server cannot read dialogs: a search of all is a search of chats.
  $('search-hint').hidden = !query || Boolean(searchPeer || searchChat);
}

// The page is asked for with who is signed in, in one round trip.
let first = query || searchPeer || searchChat ? null : started(api.dialogs(''));
me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  showSearch();
  $('new-chat').addEventListener('click', () => openNewChat(me));
  $('saved').hidden = !me.nickname;
  if (me.nickname) $('saved').href = savedPath(me);
  $('only-unread').addEventListener('click', toggleUnread);

  fitHeight();

  if (query) {
    $('more').addEventListener('click', searchMore);
    loadOnScroll(searchMore);
    await searchMore();
  } else if (searchPeer || searchChat) {
    $('search-text').focus();
  } else {
    $('more').addEventListener('click', loadMore);
    loadOnScroll(loadMore);
    await loadMore();
    listenToEvents();
  }
}
