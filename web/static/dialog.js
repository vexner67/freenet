// A dialog at /im/<nickname>, as on openvk: messages oldest first, older
// ones load when scrolled to the top, new ones and read marks come live
// (events.js) and are polled only while that is down. Enter sends,
// Shift+Enter breaks the line. Unread messages are grey either way: the
// peer's until you read them, yours until the peer does. Your own message
// can be deleted for both for a day, as "delete for everyone" on VK.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { isLive, onEvent } from './events.js';
import { errorText, formatPostDate, genitiveName } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showCounter, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { photosNode } from './posts.js';

const POLL_MS = 5000;
// How close to the top, in pixels, loads older messages.
const LOAD_OLDER_AT = 40;

const nickname = decodeURIComponent(location.pathname.slice('/im/'.length));
const list = $('messages');

let me;
let peer;
let olderCursor = '';
let newestID = '';
let loadingOlder = false;
let attachments;
// Message ID → node, so polling neither repeats a message nor misses a read.
const shown = new Map();

function messageNode(m) {
  const author = m.out ? me : peer;

  return el('div', { className: m.read ? 'im-message' : 'im-message unread' },
    el('a', { href: '/' + author.nickname }, el('img', { src: avatarURL(author, 'small'), alt: '' })),
    el('div', { className: 'im-message-content' },
      el('a', { className: 'post-name', href: '/' + author.nickname, textContent: fullName(author) }),
      m.text ? el('div', { className: 'im-message-text', textContent: m.text }) : null,
      photosNode(m.photos ?? []),
    ),
    el('div', { className: 'im-message-date' },
      formatPostDate(m.createdAt),
      canDelete(m) && el('a', { className: 'im-message-delete', textContent: 'Удалить', onclick: () => confirmDelete(m) }),
    ),
  );
}

// canDelete: your own message, until deletableUntil; the server says no
// once it passes while the page is open.
function canDelete(m) {
  return m.out && Boolean(m.deletableUntil) && Date.now() < new Date(m.deletableUntil);
}

function confirmDelete(m) {
  const modal = openModal('Удаление сообщения');
  const error = el('div', { className: 'error', hidden: true });
  const remove = button('Удалить', async () => {
    hideError(error);
    remove.disabled = true;

    try {
      await api.deleteMessage(m.id);
      drop(m.id);
      modal.close();
    } catch (e) {
      // Gone already: what the user wanted.
      if (e.status === 404) {
        drop(m.id);
        modal.close();
        return;
      }
      showError(error, e);
      remove.disabled = false;
    }
  });

  modal.body.append(error, el('div', {}, `Сообщение будет удалено и у вас, и у ${genitiveName(peer)}.`));
  modal.actions.append(remove, button('Отмена', modal.close, { secondary: true }));
}

// drop takes a deleted message off the page.
function drop(id) {
  shown.get(id)?.remove();
  shown.delete(id);
  $('empty').hidden = shown.size > 0;
}

// add puts messages at the end (newer) or the start (older), skipping
// those already shown.
function add(messages, atStart) {
  const fresh = messages.filter((m) => !shown.has(m.id));
  const nodes = fresh.map((m) => {
    const node = messageNode(m);
    node.message = m;
    shown.set(m.id, node);
    return node;
  });

  if (atStart) {
    $('empty').after(...nodes);
  } else {
    list.append(...nodes);
  }

  for (const m of fresh) {
    if (m.id > newestID) newestID = m.id;
  }

  $('empty').hidden = shown.size > 0;

  return fresh;
}

// markTheirRead greys out my messages up to what the peer has read.
// UUIDv7 strings compare as their time.
function markTheirRead(peerReadUpTo) {
  for (const node of shown.values()) {
    if (node.message.out && peerReadUpTo && node.message.id <= peerReadUpTo) {
      node.message.read = true;
      node.classList.remove('unread');
    }
  }
}

// readIncoming marks the newest message from the peer read, once the user
// can see it.
async function readIncoming(messages) {
  const incoming = messages.filter((m) => !m.out && !m.read);
  if (incoming.length === 0 || document.hidden) return;

  // The newest by ID: older pages are added after newer ones.
  const newest = incoming.reduce((a, b) => (b.id > a.id ? b : a));

  try {
    await api.markDialogRead(nickname, newest.id);
    for (const m of incoming) {
      m.read = true;
      shown.get(m.id)?.classList.remove('unread');
    }
    showCounter('messages');
  } catch {
    // Stays unread; the next poll tries again.
  }
}

const nearBottom = () => list.scrollHeight - list.scrollTop - list.clientHeight < 60;
const toBottom = () => { list.scrollTop = list.scrollHeight; };

async function loadOlder() {
  if (loadingOlder || !olderCursor) return;
  loadingOlder = true;

  try {
    const page = await api.messages(nickname, { cursor: olderCursor });
    // Keep the view still while messages appear above it.
    const fromBottom = list.scrollHeight - list.scrollTop;

    add(page.messages, true);
    olderCursor = page.nextCursor;
    list.scrollTop = list.scrollHeight - fromBottom;
  } catch (e) {
    showError($('error'), e);
  } finally {
    loadingOlder = false;
  }
}

// showNew adds new messages, keeping the view at the bottom if it was.
async function showNew(messages) {
  const stick = nearBottom();
  const fresh = add(messages, false);

  if (fresh.length > 0 && stick) toBottom();
  await readIncoming(fresh);
}

// catchUp asks for what came after the newest message shown: by polling
// while events are down, and once after they come back.
async function catchUp() {
  try {
    // An empty dialog has nothing to ask after: ask for the newest.
    const page = await api.messages(nickname, newestID ? { after: newestID } : {});

    markTheirRead(page.peerReadUpTo);
    await showNew(page.messages);
  } catch {
    // A missed poll is caught up by the next one.
  }
}

async function poll() {
  if (!document.hidden && !isLive()) await catchUp();

  setTimeout(poll, POLL_MS);
}

function listenToEvents() {
  onEvent('message', (e) => {
    if (e.peer.id === peer.id) showNew([e.message]);
  });
  onEvent('read', (e) => {
    if (e.peer.id === peer.id && !e.byMe) markTheirRead(e.upTo);
  });
  // Taken back by the peer, or by me on another page.
  onEvent('delete', (e) => {
    if (e.peer.id === peer.id) drop(e.id);
  });
  // Whatever happened while disconnected.
  onEvent('ready', catchUp);
}

async function send(event) {
  event.preventDefault();
  hideError($('error'));

  const text = $('text');
  const button = $('send');
  if (!text.value.trim() && attachments.count() === 0) return;

  if (attachments.busy()) {
    showError($('error'), 'Дождитесь, пока загрузятся фотографии.');
    return;
  }

  button.disabled = true;

  try {
    const m = await api.sendMessage(nickname, text.value, attachments.ids());

    add([m], false);
    text.value = '';
    attachments.clear();
    toBottom();
  } catch (e) {
    showError($('error'), e);
  } finally {
    button.disabled = false;
    text.focus();
  }
}

// setUpInput shows the form, or why there is none; canWrite is the
// peer's privacy and blacklists, as the first page of the dialog says.
async function setUpInput(canWrite) {
  if (me.id === peer.id) {
    $('form').hidden = true;
    $('blocked').textContent = 'Нельзя написать самому себе.';
    $('blocked').hidden = false;
    return;
  }

  if (!me.profileCompleted) {
    $('form').hidden = true;
    $('blocked').replaceChildren('Чтобы писать сообщения, ', el('a', { href: '/edit', textContent: 'заполните свою страницу' }), '.');
    $('blocked').hidden = false;
    return;
  }

  if (!canWrite) {
    $('form').hidden = true;
    $('blocked').replaceChildren(...await whyNotWrite());
    $('blocked').hidden = false;
    return;
  }

  $('my-avatar').src = avatarURL(me, 'small');
  $('peer-avatar').src = avatarURL(peer, 'small');
  attachments = initAttachments({ link: $('attach'), input: $('file'), list: $('photos'), error: $('error'), onChange: () => {} });
  $('form').addEventListener('submit', send);
  $('text').addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      $('form').requestSubmit();
    }
  });
}

// whyNotWrite tells the caller's own blacklist and a deleted page apart
// from the peer's choice, which openvk words the same for both.
async function whyNotWrite() {
  const profile = await api.profile(peer.nickname).catch(() => null);

  if (profile?.blockedByMe) {
    return [`${fullName(peer)} — в вашем `, el('a', { href: '/settings/blacklist', textContent: 'чёрном списке' }), '.'];
  }

  if (profile?.deactivated) {
    return [errorText({ message: 'this page is deleted' })];
  }

  return [errorText({ message: 'you cannot write to this user' })];
}

me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const page = await api.messages(nickname);
    peer = page.peer;

    document.title = `${fullName(peer)} | Сообщения`;
    $('title').replaceChildren(
      el('a', { href: '/im', textContent: 'Мои Сообщения' }),
      ' » ',
      el('a', { href: '/' + peer.nickname, textContent: fullName(peer) }),
    );

    $('messenger').hidden = false;
    add(page.messages, false);
    olderCursor = page.nextCursor;
    toBottom();
    await setUpInput(page.canWrite);

    list.addEventListener('scroll', () => {
      if (list.scrollTop < LOAD_OLDER_AT) loadOlder();
    });
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) readIncoming([...shown.values()].map((node) => node.message));
    });

    await readIncoming(page.messages);
    listenToEvents();
    setTimeout(poll, POLL_MS);
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Ошибка';
    showError($('error'), e.status === 404 ? 'Такого пользователя нет.' : e);
  }
}
