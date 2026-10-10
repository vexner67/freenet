// A dialog at /im/<nickname>, as on openvk: messages oldest first, older
// ones load when scrolled to the top, new ones and read marks come live
// (events.js) and are polled only while that is down. Enter sends,
// Shift+Enter breaks the line. Unread messages are grey either way: the
// peer's until you read them, yours until the peer does. As on VK: any
// message can be deleted for yourself, your own for both for a day
// ("Удалить для всех") and edited for a day; a click on a message selects
// it, and several are deleted at once.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { isLive, onEvent } from './events.js';
import { errorText, formatPostDate, genitiveName, selectedCount } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showCounter, showError, hideError, autosize } from './layout.js';
import { button, openModal } from './modal.js';
import { photosNode, sharedNode } from './posts.js';

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

// The messages selected by a click, by ID, and the one being edited.
const selected = new Set();
let editing = null;

function messageNode(m) {
  const author = m.out ? me : peer;
  const text = el('div', { className: 'im-message-text' });
  const date = el('div', { className: 'im-message-date' });

  const node = el('div', { className: m.read ? 'im-message' : 'im-message unread' },
    el('a', { href: '/' + author.nickname }, el('img', { src: avatarURL(author, 'small'), alt: '' })),
    el('div', { className: 'im-message-content' },
      el('a', { className: 'post-name', href: '/' + author.nickname, textContent: fullName(author) }),
      text,
      photosNode(m.photos ?? []),
      m.repost ? sharedNode(m.repost) : null,
    ),
    date,
  );

  // render shows what the message says now: an edit changes it.
  node.render = () => {
    const shownMessage = node.message ?? m;

    text.textContent = shownMessage.text;
    text.hidden = !shownMessage.text;
    date.replaceChildren(
      formatPostDate(shownMessage.createdAt),
      shownMessage.editedAt ? el('span', { className: 'post-edited', title: 'Отредактировано', textContent: ' (ред.)' }) : '',
      canEdit(shownMessage) && el('a', { className: 'im-message-action', textContent: 'Редактировать', onclick: () => startEdit(shownMessage) }),
      el('a', { className: 'im-message-action', textContent: 'Удалить', onclick: () => confirmDelete([shownMessage]) }),
    );
  };
  node.render();

  // A click on the message itself, not on a link or a photo in it,
  // selects it, as on VK.
  node.addEventListener('click', (event) => {
    if (event.target.closest('a, img, button')) return;
    if (window.getSelection()?.toString()) return;

    toggleSelected(m.id);
  });

  return node;
}

// canDeleteForAll: your own message, until deletableUntil; the server
// says no once it passes while the page is open.
function canDeleteForAll(m) {
  return m.out && Boolean(m.deletableUntil) && Date.now() < new Date(m.deletableUntil);
}

// canEdit: your own message, until editableUntil.
function canEdit(m) {
  return m.out && Boolean(m.editableUntil) && Date.now() < new Date(m.editableUntil);
}

function toggleSelected(id) {
  if (selected.has(id)) selected.delete(id);
  else selected.add(id);

  shown.get(id)?.classList.toggle('selected', selected.has(id));
  showSelection();
}

function clearSelection() {
  for (const id of selected) shown.get(id)?.classList.remove('selected');
  selected.clear();
  showSelection();
}

// showSelection is the bar over the dialog while messages are selected.
function showSelection() {
  $('selection').hidden = selected.size === 0;
  $('selection-count').textContent = selectedCount(selected.size);
}

// confirmDelete deletes messages for me, or, with the box ticked, for
// both: offered while every one of them is mine and still may be taken
// back.
function confirmDelete(messages) {
  const modal = openModal(messages.length > 1 ? 'Удаление сообщений' : 'Удаление сообщения');
  const error = el('div', { className: 'error', hidden: true });
  const forAll = el('input', { type: 'checkbox' });
  const remove = button('Удалить', async () => {
    hideError(error);
    remove.disabled = true;

    try {
      for (const m of messages) {
        // Gone already: what the user wanted.
        await api.deleteMessage(m.id, { forMe: !forAll.checked }).catch((e) => {
          if (e.status !== 404) throw e;
        });
        drop(m.id);
      }
      modal.close();
    } catch (e) {
      showError(error, e);
      remove.disabled = false;
    }
  });

  modal.body.append(error, el('div', {}, messages.length > 1
    ? `Вы действительно хотите удалить ${selectedCount(messages.length).replace('Выбрано ', '')}?`
    : 'Вы действительно хотите удалить это сообщение?'));
  if (messages.every(canDeleteForAll)) {
    modal.body.append(el('label', { className: 'publish-signed' }, forAll, ` Удалить для всех (и у ${genitiveName(peer)})`));
  }
  modal.actions.append(remove, button('Отмена', modal.close, { secondary: true }));
}

// drop takes a deleted message off the page.
function drop(id) {
  shown.get(id)?.remove();
  shown.delete(id);
  if (selected.delete(id)) showSelection();
  if (editing?.id === id) cancelEdit();
  $('empty').hidden = shown.size > 0;
}

// startEdit puts what the message says back in the form: sending saves
// the edit, as on VK.
function startEdit(m) {
  const text = $('text');

  editing = m;
  text.value = m.text;
  autosize(text);
  $('editing').hidden = false;
  $('send').textContent = 'Сохранить';
  $('attach').hidden = true;
  text.focus();
  text.setSelectionRange(text.value.length, text.value.length);
}

function cancelEdit() {
  if (!editing) return;

  editing = null;
  $('text').value = '';
  autosize($('text'));
  $('editing').hidden = true;
  $('send').textContent = 'Отправить';
  $('attach').hidden = false;
}

// edited shows what a message says after an edit, mine or the peer's.
function edited(m) {
  const node = shown.get(m.id);
  if (!node) return;

  node.message = { ...node.message, text: m.text, editedAt: m.editedAt };
  node.render();
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
  // Taken back by the peer, or deleted by me on another page.
  onEvent('delete', (e) => {
    if (e.peer.id === peer.id) drop(e.id);
  });
  onEvent('edit', (e) => {
    if (e.peer.id === peer.id) edited(e.message);
  });
  // Whatever happened while disconnected.
  onEvent('ready', catchUp);
}

async function send(event) {
  event.preventDefault();
  hideError($('error'));

  const text = $('text');
  const button = $('send');

  if (editing) {
    button.disabled = true;

    try {
      edited(await api.editMessage(editing.id, text.value));
      cancelEdit();
    } catch (e) {
      showError($('error'), e);
    } finally {
      button.disabled = false;
      text.focus();
    }

    return;
  }

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
    autosize(text);
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
  $('editing-cancel').addEventListener('click', cancelEdit);
  $('text').addEventListener('keydown', (event) => {
    if (event.key === 'Escape') cancelEdit();
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

    $('search-link').href = '/im?peer=' + encodeURIComponent(peer.nickname);
    $('search-link').hidden = false;
    $('selection-delete').addEventListener('click', () => {
      confirmDelete([...selected].map((id) => shown.get(id)?.message).filter(Boolean));
    });
    $('selection-cancel').addEventListener('click', clearSelection);

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
