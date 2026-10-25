// A dialog at /im/<nickname>, as on openvk: messages oldest first, older
// ones load when scrolled to the top, new ones and read marks come live
// (events.js) and are polled only while that is down. Enter sends,
// Shift+Enter breaks the line. Unread messages are grey either way: the
// peer's until you read them, yours until the peer does. As on VK: any
// message can be deleted for yourself, your own for both for a day
// ("Удалить для всех") and edited for a day; a click on a message selects
// it, and several are deleted or forwarded ("Переслать") at once, and
// "печатает..." shows while the peer types (typing.js). A message is
// answered ("Ответить") and reacted to, as on VK (replies.js). A
// message taken back stays as "Сообщение удалено". With ?msg=<id> the
// dialog opens around that message, as a search leads to it. What the
// messages say is sealed for the two people (e2ee.js): the page opens it
// with the key of this browser and seals what it sends; until the peer has
// a key nobody can write to them.

import { api, currentUser, started } from './api.js';
import { initAttachments } from './attach.js';
import { initGraffiti } from './graffiti.js';
import { initDocuments } from './docs.js';
import { editSealed, ensureKey, KEY_READY, keyState, NO_PEER_KEY, notePeerKey, openMessages, ownKey, safetyCode, sendSealed } from './e2ee.js';
import { isLive, onEvent } from './events.js';
import { actionsNode, attachedNodes, forwardedNode, openForward, selectOnDoubleClick, takenBack } from './forward.js';
import { errorText, formatPostDate, genitiveName, isLongText, keyStateText, photoChanges, messagesCount, primeDress, primeNameClass, selectedCount, votesCount } from './labels.js';
import { $, avatarURL, el, fullName, mentionNodes, renderChrome, showCounter, showError, hideError, autosize, stickToBottom, verifiedMark } from './layout.js';
import { button, openModal } from './modal.js';
import { closePushes } from './push.js';
import { quoteNode, reactionNodes, reactWith, replyBar } from './replies.js';
import { initStickers } from './stickers.js';
import { typingLine, watchTyping } from './typing.js';
import { tr } from './i18n.js';

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
// messagePrice: the votes a message to the peer costs, 0 — free.
let messagePrice = 0;
// typingWatch tells the peer while the user types.
let typingWatch;
// The key messages to the peer are sealed with, as the server has it.
let peerKey = null;
// Message ID → node, so polling neither repeats a message nor misses a read.
const shown = new Map();

// The messages selected by a click, by ID, and the one being edited.
const selected = new Set();
let editing = null;
// replies runs "Ответить" once the form is there.
let replies = null;

function messageNode(m) {
  const author = m.out ? me : peer;
  const content = el('div', { className: 'im-message-body' });
  const date = el('div', { className: 'im-message-date' });

  const node = el('div', { className: ['im-message', ...(m.read ? [] : ['unread']), ...primeDress(author)].join(' ') },
    el('a', { href: '/' + author.nickname }, el('img', { src: avatarURL(author, 'small'), alt: '' })),
    el('div', { className: 'im-message-content' },
      el('a', { className: ['post-name', primeNameClass(author)].filter(Boolean).join(' '), href: '/' + author.nickname, textContent: fullName(author) }),
      verifiedMark(author),
      content,
    ),
    date,
  );

  // render shows what the message is now: an edit changes what it says,
  // and taken back it says nothing.
  node.render = () => {
    const shownMessage = node.message ?? m;
    const text = shownMessage.deleted ? tr('Сообщение удалено') : shownMessage.text;

    node.classList.toggle('deleted', Boolean(shownMessage.deleted));
    content.replaceChildren(...[
      !shownMessage.deleted && shownMessage.unreadable
        ? el('div', { className: 'im-message-text unreadable', textContent: tr('Не удалось расшифровать сообщение: оно зашифровано ключом, которого нет на этом устройстве.') })
        : text && textOf(text, node),
      ...attachedNodes(shownMessage),
      forwardedNode(shownMessage, (carried) => textOf(carried, node)),
      ...(shownMessage.deleted ? [] : reactionNodes(shownMessage, me.id, node.picking, react)),
    ].filter(Boolean));
    if (shownMessage.replyTo && !shownMessage.deleted) content.prepend(quoteNode(shownMessage.replyTo, quotedText(shownMessage), goTo));
    date.replaceChildren(...[
      formatPostDate(shownMessage.createdAt),
      shownMessage.editedAt && el('span', { className: 'post-edited', title: tr('Отредактировано'), textContent: tr(' (ред.)') }),
      actionsNode([
        replies && !shownMessage.deleted && !shownMessage.unreadable && [tr('Ответить'), () => answer(shownMessage)],
        !shownMessage.deleted && [tr('Реакция'), () => {
          node.picking = !node.picking;
          node.render();
          // Under the last message the picker would open out of sight.
          if (node.picking) node.querySelector('.im-reaction-picker')?.scrollIntoView({ block: 'nearest' });
        }],
        canEdit(shownMessage) && [tr('Редактировать'), () => startEdit(shownMessage)],
        !shownMessage.deleted && !shownMessage.unreadable && [tr('Переслать'), () => forward([shownMessage])],
        [tr('Удалить'), () => confirmDelete([shownMessage])],
      ]),
    ].filter(Boolean));
  };
  node.message = m;
  node.render();

  // A double click on the message itself, not on a link or a photo in
  // it, selects it; then a click selects more.
  selectOnDoubleClick(node, () => toggleSelected(m.id), () => selected.size > 0);

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
    textContent: tr('Показать больше'),
    onclick: () => {
      node.opened = true;
      node.render();
    },
  });

  return el('div', {}, body, more);
}

// quotedText is the words of the message m answers: as it says now when
// it is on the page, otherwise as its sender quoted it, sealed.
function quotedText(m) {
  const original = shown.get(m.replyTo.id)?.message;
  return original && !original.unreadable && !original.deleted ? original.text : m.quoted;
}

// goTo shows a message of the dialog: where it stands when it is loaded,
// otherwise the dialog opens around it.
function goTo(id) {
  const node = shown.get(id);
  if (!node) {
    location.assign('/im/' + encodeURIComponent(nickname) + '?msg=' + encodeURIComponent(id));
    return;
  }

  for (const other of list.querySelectorAll('.im-message.found')) other.classList.remove('found');
  node.classList.add('found');
  node.scrollIntoView({ block: 'center' });
}

// answer is "Ответить": the next message sent answers m.
function answer(m) {
  cancelEdit();
  replies.start(m, m.out ? me : peer, m.text);
}

const react = (m, reaction) => reactWith({ shown, userID: me.id, put: api.reactToMessage, error: $('error') })(m, reaction);

// page is a page of the dialog with what its messages say opened.
async function page(at = {}) {
  return opened(api.messages(nickname, at));
}

// opened is the page asked, its messages opened with the keys here.
async function opened(asked) {
  const p = await asked;

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
  const modal = openModal(messages.length > 1 ? tr('Удаление сообщений') : tr('Удаление сообщения'));
  const error = el('div', { className: 'error', hidden: true });
  const forAll = el('input', { type: 'checkbox' });
  const remove = button(tr('Удалить'), async () => {
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
    ? tr('Вы действительно хотите удалить {0}?', messagesCount(messages.length))
    : tr('Вы действительно хотите удалить это сообщение?')));
  if (messages.every(canDeleteForAll)) {
    modal.body.append(el('label', { className: 'publish-signed' }, forAll, tr(' Удалить для всех (и у {0})', genitiveName(peer))));
  }
  modal.actions.append(remove, button(tr('Отмена'), modal.close, { secondary: true }));
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
  if (replies?.replyingTo()?.id === id) replies.cancel();
  $('empty').hidden = shown.size > 0;
}

// startEdit puts what the message says and its photos back in the form:
// sending saves the edit, as on VK. A message that shares a photo of an
// album takes no photos of its own.
function startEdit(m) {
  const text = $('text');

  replies.cancel();
  editing = m;
  text.value = m.text;
  autosize(text);
  attachments.set(m.photos ?? []);
  $('editing').hidden = false;
  $('send').textContent = tr('Сохранить');
  // An edit changes the words and the photos; the documents stay.
  documents.clear();
  $('doc').hidden = true;
  if (m.sharedPhoto) $('attach').hidden = $('graffiti').hidden = true;
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
  $('send').textContent = tr('Отправить');
}

// edited shows what a message is after its sender, me or the peer,
// edited it or took it back; whether it is read the event does not tell.
function edited(m) {
  const node = shown.get(m.id);
  if (!node) return;

  node.message = { ...node.message, ...m, read: node.message.read };
  node.render();
  if (editing?.id === m.id && m.deleted) cancelEdit();
  if (replies?.replyingTo()?.id === m.id && m.deleted) replies.cancel();
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
    closePushes('im:' + peer.id).catch(() => {});
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
  const asked = early;
  early = null;
  if (!foundID) return opened(asked);

  try {
    const around = await opened(asked);

    gap = true;
    return around;
  } catch (e) {
    if (e.status !== 404 && e.status !== 400) throw e;
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
      showError($('error'), tr('Дождитесь, пока загрузятся фотографии.'));
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
    showError($('error'), tr('Дождитесь, пока загрузятся фотографии.'));
    return;
  }

  button.disabled = true;

  try {
    const m = await sendSealed(me.id, nickname, text.value, { photos: attachments.ids(), documents: documents.ids(), replyTo: replies.replyingTo(), price: messagePrice });

    text.value = '';
    autosize(text);
    replies.cancel();
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
// nothing. It tells whether it was sent.
async function sendSticker(sticker) {
  hideError($('error'));
  if (editing) cancelEdit();

  try {
    showSent((await openMessages(me.id, [await api.sendSticker(nickname, sticker, messagePrice)]))[0]);
    return true;
  } catch (e) {
    showError($('error'), e);
    return false;
  }
}

// setUpInput shows the form, or why there is none: the peer's privacy and
// blacklists, as the first page of the dialog says, with what a message
// costs and whether only Freenet Prime writes to the peer.
async function setUpInput({ canWrite, primeOnly, messagePrice: price = 0 }) {
  if (!me.profileCompleted) {
    $('form').hidden = true;
    $('blocked').replaceChildren(tr('Чтобы писать сообщения, '), el('a', { href: '/edit', textContent: tr('заполните свою страницу') }), '.');
    $('blocked').hidden = false;
    return;
  }

  if (!canWrite) {
    $('form').hidden = true;
    $('blocked').replaceChildren(...(primeOnly ? primeOnlyHint() : await whyNotWrite()));
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

  messagePrice = price;
  if (price > 0) {
    $('price-note').textContent = tr('Каждое сообщение для {0} стоит {1}: голоса получит собеседник.', genitiveName(peer), votesCount(price));
    $('price-note').hidden = false;
  }

  $('my-avatar').src = avatarURL(me, 'small');
  $('peer-avatar').src = avatarURL(peer, 'small');
  attachments = initAttachments({ link: $('attach'), also: [$('graffiti')], input: $('file'), list: $('photos'), error: $('error'), onChange: () => {} });
  initGraffiti({ link: $('graffiti'), attachments });
  documents = initDocuments({ link: $('doc'), list: $('documents') });
  initStickers({ link: $('stickers-link'), panel: $('stickers'), error: $('error'), onPick: sendSticker });
  $('form').addEventListener('submit', send);
  $('editing-cancel').addEventListener('click', cancelEdit);
  replies = replyBar();
  // The messages shown already get "Ответить".
  for (const node of shown.values()) node.render();
  $('text').addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
      cancelEdit();
      replies.cancel();
    }
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      $('form').requestSubmit();
    }
  });
  // Editing a message is not writing one; nobody reads «Избранное» but
  // the one who writes.
  if (saved()) return;
  typingWatch = watchTyping($('text'), () => (editing ? Promise.resolve() : api.dialogTyping(peer.nickname)));
}

// saved tells whether this is «Избранное», the dialog with oneself: sealed
// with one's own key, with no one to compare a code with.
function saved() {
  return peer?.id === me?.id;
}

// keyHint is why this browser can neither open nor seal messages, with
// where to bring the key.
function keyHint(state) {
  return [keyStateText(state) + ' ', el('a', { href: '/settings?act=security', textContent: tr('Перейти к настройкам шифрования') }), '.'];
}

// showKeys learns the peer's key and says when it is not the one this
// browser saw before: a new device of theirs, or someone between.
async function showKeys() {
  peerKey = (await api.messageKey(peer.nickname)).publicKey;
  if (!peerKey || saved()) return;

  if (notePeerKey(me.id, peer.id, peerKey) === 'changed') {
    $('key-notice').replaceChildren(
      tr('Ключ шифрования собеседника изменился: возможно, {0} настраивает новое устройство. ', fullName(peer)),
      el('a', { textContent: tr('Сравните код безопасности'), onclick: showSafetyCode }),
      '.',
    );
    $('key-notice').hidden = false;
  }
}

// showSafetyCode is the code both people compare, each in their dialog,
// to be sure no one reads between them.
async function showSafetyCode() {
  const modal = openModal(tr('Шифрование сообщений'));
  const own = ownKey(me.id);

  modal.body.append(el('p', {}, tr('Сообщения этого диалога зашифрованы: прочитать их можете только вы и '),
    fullName(peer), tr(', даже сервер их не видит.')));
  if (own && peerKey) {
    modal.body.append(
      el('p', {}, tr('Код безопасности:')),
      el('p', { className: 'safety-code', textContent: await safetyCode(own, peerKey) }),
      el('p', { className: 'hint', textContent: tr('Сравните его с кодом в этом диалоге у собеседника — при встрече или по другому каналу. Если коды совпадают, вашу переписку никто не подменяет.') }),
    );
  } else {
    modal.body.append(el('p', { className: 'hint', textContent: own ? NO_PEER_KEY : keyStateText(keyState(me.id) || 'missing') }));
  }
  modal.actions.append(button(tr('Закрыть'), modal.close));
}

// whyNotWrite tells the caller's own blacklist and a deleted page apart
// from the peer's choice, which openvk words the same for both.
// primeOnlyHint: only people with Freenet Prime write to the peer.
function primeOnlyHint() {
  return [tr('{0} принимает сообщения только от подписчиков ', fullName(peer)), el('a', { href: '/prime', textContent: 'Freenet Prime' }), '.'];
}

async function whyNotWrite() {
  const profile = await api.profile(peer.nickname).catch(() => null);

  if (profile?.blockedByMe) {
    return [tr('{0} — в вашем ', fullName(peer)), el('a', { href: '/settings/blacklist', textContent: tr('чёрном списке') }), '.'];
  }

  if (profile?.deactivated) {
    return [errorText({ message: 'this page is deleted' })];
  }

  return [errorText({ message: 'you cannot write to this user' })];
}

// The page is asked for with who is signed in, in one round trip.
// The messages first shown: the newest, or those around the one found.
let early = started(api.messages(nickname, foundID ? { around: foundID } : {}));
me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const first = await firstPage();
    peer = first.peer;

    document.title = saved() ? tr('Избранное') : tr('{0} | Сообщения', fullName(peer));
    $('title').replaceChildren(
      el('a', { href: '/im', textContent: tr('Мои Сообщения') }),
      ' » ',
      ...(saved()
        ? [tr('Избранное')]
        : [el('a', { href: '/' + peer.nickname, textContent: fullName(peer) }), verifiedMark(peer)]),
    );

    $('search-link').href = '/im?peer=' + encodeURIComponent(peer.nickname);
    $('safety-link').addEventListener('click', showSafetyCode);
    if (saved()) $('dialog-links').replaceChildren($('search-link'));
    $('dialog-links').hidden = false;
    const selectedMessages = () => [...selected].map((id) => shown.get(id)?.message).filter(Boolean);

    $('selection-forward').addEventListener('click', () => forward(selectedMessages()));
    $('selection-delete').addEventListener('click', () => confirmDelete(selectedMessages()));
    $('selection-cancel').addEventListener('click', clearSelection);

    await showKeys().catch(() => {});
    $('messenger').hidden = false;
    add(first.messages, false);
    olderCursor = first.nextCursor;
    // At the end, and kept there while the messages take their height.
    const stick = stickToBottom(list);
    await setUpInput(first);
    if (gap) {
      stick(false);
      await showFound();
    }
    // The field is ready to type in as the conversation opens, as on VK;
    // without scrolling, so the list keeps its place.
    if ($('text').offsetParent) $('text').focus({ preventScroll: true });

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
    $('title').textContent = e.status === 404 ? tr('Страница не найдена') : tr('Ошибка');
    showError($('error'), e.status === 404 ? tr('Такого пользователя нет.') : e);
  }
}
