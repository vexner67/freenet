// A dialog at /im/<nickname>, as on openvk: messages oldest first, older
// ones load when scrolled to the top, new ones and read marks come live
// (events.js) and are polled only while that is down. Enter sends,
// Shift+Enter breaks the line. Unread messages are grey either way: the
// peer's until you read them, yours until the peer does. As on VK: any
// message can be deleted for yourself, your own for both for a day
// ("Удалить для всех") and edited for a day; a click on a message selects
// it, and several are deleted or forwarded ("Переслать") at once, and
// "печатает..." shows while the peer types (typing.js). A
// message taken back stays as "Сообщение удалено". With ?msg=<id> the
// dialog opens around that message, as a search leads to it. What the
// messages say is sealed for the two people (e2ee.js): the page opens it
// with the key of this browser and seals what it sends; until the peer has
// a key nobody can write to them.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { initDocuments } from './docs.js';
import { editSealed, ensureKey, KEY_READY, keyState, NO_PEER_KEY, notePeerKey, openMessages, ownKey, safetyCode, sendSealed } from './e2ee.js';
import { isLive, onEvent } from './events.js';
import { attachedNodes, forwardedNode, openForward, takenBack } from './forward.js';
import { errorText, formatPostDate, genitiveName, isLongText, keyStateText, photoChanges, selectedCount } from './labels.js';
import { $, avatarURL, el, fullName, mentionNodes, renderChrome, showCounter, showError, hideError, autosize } from './layout.js';
import { button, openModal } from './modal.js';
import { initStickers } from './stickers.js';
import { typingLine, watchTyping } from './typing.js';

const POLL_MS = 5000;
const typists = typingLine((text) => { $('typing').textContent = text; });
// How many messages one request deletes.
const DELETED_AT_ONCE = 100;
// How close to the top, in pixels, loads older messages.
const LOAD_OLDER_AT = 40;

const nickname = decodeURIComponent(location.pathname.slice('/im/'.length));
const list = $('messages');
// The message to open the dialog around, from a search.
const foundID = new URLSearchParams(location.search).get('msg') ?? '';
// What the search looked for: these words are marked in that message.
const foundQuery = (new URLSearchParams(location.search).get('q') ?? '').trim();

let me;
let peer;
let olderCursor = '';
let newestID = '';
let loadingOlder = false;
// gap: the dialog was opened around an old message and the newer ones are
// not all loaded yet; they load as the user scrolls down, and until then
// live messages wait their turn.
let gap = false;
let loadingNewer = false;
let attachments;
// documents are those of "Мои Документы" attached to the message.
let documents;
// typingWatch tells the peer while the user types.
let typingWatch;
// The key messages to the peer are sealed with, as the server has it.
let peerKey = null;
// Message ID → node, so polling neither repeats a message nor misses a read.
const shown = new Map();

// The messages selected by a click, by ID, and the one being edited.
const selected = new Set();
let editing = null;

function messageNode(m) {
  const author = m.out ? me : peer;
  const content = el('div', { className: 'im-message-body' });
  const date = el('div', { className: 'im-message-date' });

  const node = el('div', { className: m.read ? 'im-message' : 'im-message unread' },
    el('a', { href: '/' + author.nickname }, el('img', { src: avatarURL(author, 'small'), alt: '' })),
    el('div', { className: 'im-message-content' },
      el('a', { className: 'post-name', href: '/' + author.nickname, textContent: fullName(author) }),
      content,
    ),
    date,
  );

  // render shows what the message is now: an edit changes what it says,
  // and taken back it says nothing.
  node.render = () => {
    const shownMessage = node.message ?? m;
    const text = shownMessage.deleted ? 'Сообщение удалено' : shownMessage.text;

    node.classList.toggle('deleted', Boolean(shownMessage.deleted));
    content.replaceChildren(...[
      !shownMessage.deleted && shownMessage.unreadable
        ? el('div', { className: 'im-message-text unreadable', textContent: 'Не удалось расшифровать сообщение: оно зашифровано ключом, которого нет на этом устройстве.' })
        : text && textOf(text, node),
      ...attachedNodes(shownMessage),
      forwardedNode(shownMessage, (carried) => textOf(carried, node)),
    ].filter(Boolean));
    date.replaceChildren(...[
      formatPostDate(shownMessage.createdAt),
      shownMessage.editedAt && el('span', { className: 'post-edited', title: 'Отредактировано', textContent: ' (ред.)' }),
      canEdit(shownMessage) && el('a', { className: 'im-message-action', textContent: 'Редактировать', onclick: () => startEdit(shownMessage) }),
      !shownMessage.deleted && !shownMessage.unreadable && el('a', { className: 'im-message-action', textContent: 'Переслать', onclick: () => forward([shownMessage]) }),
      el('a', { className: 'im-message-action', textContent: 'Удалить', onclick: () => confirmDelete([shownMessage]) }),
    ].filter(Boolean));
  };
  node.message = m;
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

// textOf shows a text of a message of node, its own or of one it carries
// on: a long one cut short until "Показать больше", as posts are. Sealed,
// a message comes whole.
function textOf(text, node) {
  const marked = node.message?.id === foundID && foundQuery;
  const body = el('div', { className: 'im-message-text' }, ...mentionNodes(text, marked ? foundQuery : ''));
  if (node.opened || !isLongText(text)) return body;

  body.classList.add('collapsed');

  const more = el('a', {
    className: 'post-text-more',
    textContent: 'Показать больше',
    onclick: () => {
      node.opened = true;
      node.render();
    },
  });

  return el('div', {}, body, more);
}

// page is a page of the dialog with what its messages say opened.
async function page(at = {}) {
  const p = await api.messages(nickname, at);

  await openMessages(me.id, p.messages);
  return p;
}

// canDeleteForAll: your own message, until deletableUntil; the server
// says no once it passes while the page is open.
function canDeleteForAll(m) {
  return m.out && !m.deleted && Boolean(m.deletableUntil) && Date.now() < new Date(m.deletableUntil);
}

// canEdit: your own message, until editableUntil, that this browser can
// open; a sticker is not edited.
function canEdit(m) {
  return m.out && !m.deleted && !m.unreadable && !m.sticker && Boolean(m.editableUntil) && Date.now() < new Date(m.editableUntil);
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
      const forMe = !forAll.checked;
      // One request deletes them all or none: when one of them is gone
      // already, the rest go one by one.
      const gone = (e) => {
        if (e.status !== 404) throw e;
      };
      const oneByOne = async () => {
        for (const m of messages) await api.deleteMessage(m.id, { forMe }).catch(gone);
      };

      for (let i = 0; i < messages.length; i += DELETED_AT_ONCE) {
        const ids = messages.slice(i, i + DELETED_AT_ONCE).map((m) => m.id);
        if (ids.length === 1) await api.deleteMessage(ids[0], { forMe }).catch(gone);
        else await api.deleteMessages(ids, { forMe }).catch((e) => (e.status === 404 ? oneByOne() : Promise.reject(e)));
      }

      for (const m of messages) {
        // Taken back from both, it stays in its place saying so.
        if (forAll.checked) edited(takenBack(m));
        else drop(m.id);
      }
      clearSelection();
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

// forward carries messages on ("Переслать"), into this dialog or another
// conversation.
function forward(messages) {
  openForward({ me, messages: messages.filter((m) => !m.unreadable), here: { peer }, authorOf: (m) => (m.out ? me : peer), onSent: showSent, onDone: clearSelection });
}

// showSent puts a message just sent at the end of the dialog; opened
// around an old message, the dialog goes to its end first.
function showSent(m) {
  if (gap) {
    location.assign('/im/' + encodeURIComponent(nickname));
    return;
  }

  add([m], false);
  toBottom();
}

// drop takes a deleted message off the page.
function drop(id) {
  shown.get(id)?.remove();
  shown.delete(id);
  if (selected.delete(id)) showSelection();
  if (editing?.id === id) cancelEdit();
  $('empty').hidden = shown.size > 0;
}

// startEdit puts what the message says and its photos back in the form:
// sending saves the edit, as on VK. A message that shares a photo of an
// album takes no photos of its own.
function startEdit(m) {
  const text = $('text');

  editing = m;
  text.value = m.text;
  autosize(text);
  attachments.set(m.photos ?? []);
  $('editing').hidden = false;
  $('send').textContent = 'Сохранить';
  // An edit changes the words and the photos; the documents stay.
  documents.clear();
  $('doc').hidden = true;
  if (m.sharedPhoto) $('attach').hidden = true;
  text.focus();
  text.setSelectionRange(text.value.length, text.value.length);
}

function cancelEdit() {
  if (!editing) return;

  editing = null;
  $('text').value = '';
  autosize($('text'));
  attachments.clear();
  $('doc').hidden = false;
  $('editing').hidden = true;
  $('send').textContent = 'Отправить';
}

// edited shows what a message is after its sender, me or the peer,
// edited it or took it back; whether it is read the event does not tell.
function edited(m) {
  const node = shown.get(m.id);
  if (!node) return;

  node.message = { ...node.message, ...m, read: node.message.read };
  node.render();
  if (editing?.id === m.id && m.deleted) cancelEdit();
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
    const older = await page({ cursor: olderCursor });
    // Keep the view still while messages appear above it.
    const fromBottom = list.scrollHeight - list.scrollTop;

    add(older.messages, true);
    olderCursor = older.nextCursor;
    list.scrollTop = list.scrollHeight - fromBottom;
  } catch (e) {
    showError($('error'), e);
  } finally {
    loadingOlder = false;
  }
}

// loadNewer continues a dialog opened around an old message towards its
// end; with nothing newer left the dialog is whole and live again.
async function loadNewer() {
  if (loadingNewer || !gap) return;
  loadingNewer = true;

  try {
    const newer = await page({ after: newestID });

    markTheirRead(newer.peerReadUpTo);
    if (newer.messages.length === 0) gap = false;
    await readIncoming(add(newer.messages, false));
  } catch (e) {
    showError($('error'), e);
  } finally {
    loadingNewer = false;
  }
}

// fillNewer loads on while the end of the list is in view: a scroll
// that reached it, or a list too short to scroll at all.
async function fillNewer() {
  while (gap && nearBottom()) {
    const before = newestID;

    await loadNewer();
    if (gap && newestID === before) break; // busy or failed: the next scroll tries again
  }
}

// showFound brings the message the dialog was opened around into view
// and marks it.
async function showFound() {
  const node = shown.get(foundID);

  // The words may be said after what a collapsed text shows: it opens.
  if (node && foundQuery && node.querySelector('.im-message-text.collapsed')) {
    node.opened = true;
    node.render();
  }

  node?.classList.add('found');
  (node?.querySelector('.found-words') ?? node)?.scrollIntoView({ block: 'center' });
  await fillNewer();
}

// firstPage is the dialog around the found message, or its newest
// messages: without one, or when it is not there anymore.
async function firstPage() {
  if (foundID) {
    try {
      const around = await page({ around: foundID });

      gap = true;
      return around;
    } catch (e) {
      if (e.status !== 404 && e.status !== 400) throw e;
    }
  }

  return page();
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
  // Opened around an old message, the dialog catches up by scrolling.
  if (gap) return;

  try {
    // An empty dialog has nothing to ask after: ask for the newest.
    const newer = await page(newestID ? { after: newestID } : {});

    markTheirRead(newer.peerReadUpTo);
    await showNew(newer.messages);
  } catch {
    // A missed poll is caught up by the next one.
  }
}

async function poll() {
  if (!document.hidden && !isLive()) await catchUp();

  setTimeout(poll, POLL_MS);
}

function listenToEvents() {
  onEvent('message', async (e) => {
    if (e.peer.id !== peer.id) return;
    if (!e.message.out) typists.stopped(peer.id);
    if (gap) return;

    await openMessages(me.id, [e.message]);
    showNew([e.message]);
  });
  onEvent('read', (e) => {
    if (e.peer.id === peer.id && !e.byMe) markTheirRead(e.upTo);
  });
  // Taken back by the peer, or deleted by me on another page.
  onEvent('delete', (e) => {
    if (e.peer.id === peer.id) drop(e.id);
  });
  onEvent('edit', async (e) => {
    if (e.peer.id !== peer.id) return;

    await openMessages(me.id, [e.message]);
    edited(e.message);
  });
  onEvent('typing', (e) => {
    if (!e.chat && e.user.id === peer.id) typists.typing(e.user);
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
    if (attachments.busy()) {
      showError($('error'), 'Дождитесь, пока загрузятся фотографии.');
      return;
    }

    button.disabled = true;

    try {
      edited(await editSealed(me.id, nickname, editing, text.value, photoChanges(editing.photos, attachments.ids())));
      cancelEdit();
    } catch (e) {
      showError($('error'), e);
    } finally {
      button.disabled = false;
      text.focus();
    }

    return;
  }

  if (!text.value.trim() && attachments.count() === 0 && documents.count() === 0) return;

  if (attachments.busy()) {
    showError($('error'), 'Дождитесь, пока загрузятся фотографии.');
    return;
  }

  button.disabled = true;

  try {
    const m = await sendSealed(me.id, nickname, text.value, { photos: attachments.ids(), documents: documents.ids() });

    text.value = '';
    autosize(text);
    typingWatch.sent();
    attachments.clear();
    documents.clear();
    showSent(m);
  } catch (e) {
    showError($('error'), e);
  } finally {
    button.disabled = false;
    text.focus();
  }
}

// sendSticker sends a sticker by itself: it is not sealed, it says
// nothing.
async function sendSticker(sticker) {
  hideError($('error'));
  if (editing) cancelEdit();

  try {
    showSent((await openMessages(me.id, [await api.sendSticker(nickname, sticker)]))[0]);
  } catch (e) {
    showError($('error'), e);
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

  // Until both have keys, nothing can be sealed.
  const ownState = await ensureKey(me.id).catch(() => KEY_READY);
  if (ownState !== KEY_READY || !peerKey) {
    $('form').hidden = true;
    $('blocked').replaceChildren(...(ownState !== KEY_READY ? keyHint(ownState) : [NO_PEER_KEY]));
    $('blocked').hidden = false;
    return;
  }

  $('my-avatar').src = avatarURL(me, 'small');
  $('peer-avatar').src = avatarURL(peer, 'small');
  attachments = initAttachments({ link: $('attach'), input: $('file'), list: $('photos'), error: $('error'), onChange: () => {} });
  documents = initDocuments({ link: $('doc'), list: $('documents') });
  initStickers({ link: $('stickers-link'), panel: $('stickers'), error: $('error'), onPick: sendSticker });
  $('form').addEventListener('submit', send);
  $('editing-cancel').addEventListener('click', cancelEdit);
  $('text').addEventListener('keydown', (event) => {
    if (event.key === 'Escape') cancelEdit();
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      $('form').requestSubmit();
    }
  });
  // Editing a message is not writing one.
  typingWatch = watchTyping($('text'), () => (editing ? Promise.resolve() : api.dialogTyping(peer.nickname)));
}

// keyHint is why this browser can neither open nor seal messages, with
// where to bring the key.
function keyHint(state) {
  return [keyStateText(state) + ' ', el('a', { href: '/settings?act=security', textContent: 'Перейти к настройкам шифрования' }), '.'];
}

// showKeys learns the peer's key and says when it is not the one this
// browser saw before: a new device of theirs, or someone between.
async function showKeys() {
  peerKey = (await api.messageKey(peer.nickname)).publicKey;
  if (!peerKey) return;

  if (notePeerKey(me.id, peer.id, peerKey) === 'changed') {
    $('key-notice').replaceChildren(
      `Ключ шифрования собеседника изменился: возможно, ${fullName(peer)} настраивает новое устройство. `,
      el('a', { textContent: 'Сравните код безопасности', onclick: showSafetyCode }),
      '.',
    );
    $('key-notice').hidden = false;
  }
}

// showSafetyCode is the code both people compare, each in their dialog,
// to be sure no one reads between them.
async function showSafetyCode() {
  const modal = openModal('Шифрование сообщений');
  const own = ownKey(me.id);

  modal.body.append(el('p', {}, 'Сообщения этого диалога зашифрованы: прочитать их можете только вы и ',
    fullName(peer), ', даже сервер их не видит.'));
  if (own && peerKey) {
    modal.body.append(
      el('p', {}, 'Код безопасности:'),
      el('p', { className: 'safety-code', textContent: await safetyCode(own, peerKey) }),
      el('p', { className: 'hint', textContent: 'Сравните его с кодом в этом диалоге у собеседника — при встрече или по другому каналу. Если коды совпадают, вашу переписку никто не подменяет.' }),
    );
  } else {
    modal.body.append(el('p', { className: 'hint', textContent: own ? NO_PEER_KEY : keyStateText(keyState(me.id) || 'missing') }));
  }
  modal.actions.append(button('Закрыть', modal.close));
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
    const first = await firstPage();
    peer = first.peer;

    document.title = `${fullName(peer)} | Сообщения`;
    $('title').replaceChildren(
      el('a', { href: '/im', textContent: 'Мои Сообщения' }),
      ' » ',
      el('a', { href: '/' + peer.nickname, textContent: fullName(peer) }),
    );

    $('search-link').href = '/im?peer=' + encodeURIComponent(peer.nickname);
    $('safety-link').addEventListener('click', showSafetyCode);
    $('dialog-links').hidden = false;
    const selectedMessages = () => [...selected].map((id) => shown.get(id)?.message).filter(Boolean);

    $('selection-forward').addEventListener('click', () => forward(selectedMessages()));
    $('selection-delete').addEventListener('click', () => confirmDelete(selectedMessages()));
    $('selection-cancel').addEventListener('click', clearSelection);

    await showKeys().catch(() => {});
    $('messenger').hidden = false;
    add(first.messages, false);
    olderCursor = first.nextCursor;
    toBottom();
    await setUpInput(first.canWrite);
    if (gap) await showFound();

    list.addEventListener('scroll', () => {
      if (list.scrollTop < LOAD_OLDER_AT) loadOlder();
      fillNewer();
    });
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) readIncoming([...shown.values()].map((node) => node.message));
    });

    await readIncoming(first.messages);
    listenToEvents();
    setTimeout(poll, POLL_MS);
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Ошибка';
    showError($('error'), e.status === 404 ? 'Такого пользователя нет.' : e);
  }
}
