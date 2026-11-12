// A chat of several people ("беседа") at /im/chat/<id>: messages oldest
// first, older ones load when scrolled to the top, new ones come live
// (events.js) and are polled only while that is down. Between the messages
// stand the lines of what happened: who created the chat, renamed it, was
// added or removed, left or came back. Anyone in the chat writes, adds
// people and renames it; only its creator removes them ("Исключить из
// беседы", in "О чате"). Whoever left ("Покинуть чат") keeps the chat as it
// was and may come back; whoever was removed comes back only when added
// again. Your own message is edited and taken back within a day; any
// message is carried on ("Переслать"), and a click selects several to
// carry on at once. With ?msg=<id> the chat opens around that message, as
// a search leads to it. While others type, "печатает..." shows under the
// messages (typing.js). Anyone in the chat pins a message above it
// ("Закрепить", as VK: one at a time) and sets its photo, in "О чате".
// A message is answered ("Ответить") and reacted to, as on VK
// (replies.js). The window is openvk's messenger (messenger.js), and "О
// чате" is its page in place of the messages: the photo and the name, the
// actions in rows and who is in the chat.

import { api, currentUser, started } from './api.js';
import { initAttachments } from './attach.js';
import { initGraffiti } from './graffiti.js';
import { initDocuments } from './docs.js';
import { initAudios } from './audio.js';
import { initVideos } from './video.js';
import { openAddMembers } from './chats.js';
import { isLive, onEvent } from './events.js';
import { openAvatarDialog } from './avatar.js';
import { initEmoji } from './emoji.js';
import { actionsNode, attachedNodes, forwardedNode, openForward, selectOnDoubleClick, takenBack } from './forward.js';
import { chatActionParts, errorText, chatMembers, chatStateText, chatTitle, formatPostDate, isLongText, messagePreview, messagesCount, messageTime, photoChanges, primeNameClass, selectedCount, snippet, splitVideos, withVideos } from './labels.js';
import { $, avatarURL, chatPhotoURL, el, fullName, isPhone, keepFocus, whenShown, mentionNodes, renderChrome, smileyNodes, showCounter, showError, hideError, autosize, stickToBottom, verifiedMark } from './layout.js';
import { button, openModal } from './modal.js';
import { contactInfo, fitHeight, hoverIcons, peerTabs, regroup, selectOnClick, showContact } from './messenger.js';
import { closePushes } from './push.js';
import { quoteNode, reactionNodes, reactWith, replyBar, showEditing, showJumped } from './replies.js';
import { initStickers } from './stickers.js';
import { typingLine, watchTyping } from './typing.js';
import { tr } from './i18n.js';

const POLL_MS = 5000;
const typists = typingLine((text) => { $('typing').textContent = text; });
// How close to the top, in pixels, loads older messages.
const LOAD_OLDER_AT = 40;

const chatID = decodeURIComponent(location.pathname.slice('/im/chat/'.length));
const list = $('messages');
// The message to open the chat around, from a search, and what the
// search looked for: these words are marked in that message.
const foundID = new URLSearchParams(location.search).get('msg') ?? '';
const foundQuery = (new URLSearchParams(location.search).get('q') ?? '').trim();

let me;
let chat;
let olderCursor = '';
let newestID = '';
let loadingOlder = false;
// gap: the chat was opened around an old message and the newer ones are
// not all loaded yet; they load as the user scrolls down, and until then
// live messages wait their turn.
let gap = false;
let loadingNewer = false;
let attachments;
// documents are those of "Мои Документы" attached to the message.
let documents;
// audios are songs attached to the message.
let audios;
// videos are videos of YouTube attached to the message, sent as lines of
// its words (withVideos).
let videos;
// typingWatch tells the others while the user types.
let typingWatch;
let editing = null;
// replies runs "Ответить" once the form is there.
let replies = null;
// What the message said when its edit began: saving names it, so an edit
// made in another window meanwhile is not overwritten.
let editingBase = '';
// renameTab gives the tab of the chat its new name.
let renameTab = null;
// Message ID → node, so polling does not repeat a message.
const shown = new Map();
// The messages selected by a click, by ID.
const selected = new Set();


// actionNode is a line of what happened in the chat, as openvk's
// renderSpecialMessage: "Анна пригласила Петра Петрова 0:27" in the middle,
// the names bold, the rest grey and bold, the time small after it.
function actionNode(m) {
  const [before, whom, after] = chatActionParts(m);
  const name = (user, text) => el('a', { className: 'im-special-name', href: '/' + user.nickname }, el('strong', { textContent: text }));

  const node = el('div', { className: 'im-special-message' },
    el('div', {},
      name(m.author, fullName(m.author) + ' '),
      el('span', { className: 'im-special-text', textContent: before }),
      whom && name(m.target, whom),
      after && el('span', { className: 'im-special-text', textContent: after }),
      el('span', { className: 'im-special-date', title: formatPostDate(m.createdAt), textContent: messageTime(m.createdAt) }),
    ),
  );
  node.message = m;
  return node;
}

function messageNode(m) {
  if (m.action) return actionNode(m);

  const content = el('div', { className: 'im-message-body' });
  const head = el('div', { className: 'im-message-head' });
  // As on openvk: the mark of a selected message at the left, the photo,
  // the icons and the time at the right, the name and the words.
  // Grey while unread, as on openvk: someone else's I have not read, or
  // mine nobody else has read yet.
  const mine = m.author.id === me.id;
  const node = el('div', { className: ['im-message', ...(mine ? ['out'] : []), ...((mine ? !m.seen : m.unread) ? ['unread'] : [])].join(' ') },
    el('span', { className: 'im-check' }),
    el('a', { className: 'im-message-avatar', href: '/' + m.author.nickname, ariaLabel: fullName(m.author) }, el('img', { src: avatarURL(m.author, 'small'), alt: '' })),
    el('div', { className: 'im-message-content' },
      head,
      el('span', { className: 'im-sender' },
        el('a', { className: ['im-message-sender', primeNameClass(m.author)].filter(Boolean).join(' '), href: '/' + m.author.nickname, textContent: fullName(m.author) }),
        verifiedMark(m.author)), content),
  );

  // render shows what the message is now: an edit changes what it says,
  // and taken back it says nothing.
  node.render = () => {
    const now = node.message;

    node.classList.toggle('deleted', now.deleted);
    content.replaceChildren(...[
      !now.deleted && now.replyTo && quoteNode(now.replyTo, now.replyTo.text, goTo),
      now.deleted ? el('div', { className: 'im-message-text', textContent: tr('Сообщение удалено') }) : splitVideos(now.text).text && textOf(splitVideos(now.text).text, node),
      now.editedAt && !now.deleted && el('div', { className: 'im-message-edited', title: tr('Отредактировано'), textContent: tr(' (ред.)') }),
      ...attachedNodes(now),
      forwardedNode(now, (carried) => textOf(carried, node)),
      ...(now.deleted ? [] : reactionNodes(now, me.id, node.picking, react)),
    ].filter(Boolean));
    head.replaceChildren(...[
      messageActions(now, node),
      el('span', { className: 'im-message-time', title: formatPostDate(now.createdAt), textContent: messageTime(now.createdAt) }),
    ].filter(Boolean));
  };
  node.message = m;
  node.render();

  // A click on the message itself, not on a link or a photo in it,
  // selects it, as on openvk; a phone selects by a double tap and acts
  // by "⋯".
  if (isPhone()) selectOnDoubleClick(node, () => toggleSelected(m.id), () => selected.size > 0);
  else selectOnClick(node, () => toggleSelected(m.id));

  return node;
}

// canEdit: my own message of a chat I am in, until editableUntil; a
// sticker is not edited.
function canEdit(m) {
  return m.out && !m.deleted && chat.state === 'active' && !m.sticker && Boolean(m.editableUntil) && new Date(m.editableUntil) > new Date();
}

// canDelete: my own message of a chat I am in, until deletableUntil.
function canDelete(m) {
  return m.out && !m.deleted && chat.state === 'active' && Boolean(m.deletableUntil) && new Date(m.deletableUntil) > new Date();
}

// messageActions are what may be done with a message by itself: openvk's
// icons on hover — answer, react, edit — and on a phone its list under
// "⋯"; pinning, who has read it, deleting and forwarding go through the
// selection, as on openvk.
function messageActions(m, node) {
  const active = chat.state === 'active';
  const canAnswer = replies && !m.deleted && active;
  const pick = () => {
    node.picking = !node.picking;
    node.render();
    // Under the last message the picker would open out of sight.
    if (node.picking) node.querySelector('.im-reaction-picker')?.scrollIntoView({ block: 'nearest' });
  };

  if (!isPhone()) {
    return hoverIcons([
      canAnswer && [tr('Ответить'), () => answer(m), 'reply'],
      !m.deleted && active && [tr('Реакция'), pick, 'react'],
      canEdit(m) && [tr('Редактировать'), () => startEdit(m), 'edit'],
    ]);
  }

  return actionsNode([
    canAnswer && [tr('Ответить'), () => answer(m)],
    !m.deleted && active && [tr('Реакция'), pick],
    canEdit(m) && [tr('Редактировать'), () => startEdit(m)],
    !m.deleted && [tr('Переслать'), () => forward([m])],
    canDelete(m) && [tr('Удалить'), () => confirmDelete([m])],
  ]);
}

// answer is "Ответить": the next message sent answers m.
function answer(m) {
  cancelEdit();
  replies.start(m, m.author, m.text);
}

const react = (m, reaction) => reactWith({ shown, userID: me.id, put: api.reactToChatMessage, error: $('error') })(m, reaction);

// CUT_LENGTH is how much of a long text a chat carries; the message says
// only that some text of it is cut (textCut), not which.
const CUT_LENGTH = 1000;

// textOf shows a text of the message of node, its own or of one it
// carries on: a long one cut short until "Показать больше", which first
// loads the message whole when the chat carried its beginning.
function textOf(text, node) {
  const m = node.message;
  const marked = m.id === foundID && foundQuery;
  const body = el('div', { className: 'im-message-text' }, ...mentionNodes(text, marked ? foundQuery : '', false, true));
  const cut = m.textCut && [...text].length >= CUT_LENGTH;
  if (node.opened || !(cut || isLongText(text))) return body;

  body.classList.add('collapsed');

  return el('div', {}, body, el('a', {
    className: 'post-text-more',
    textContent: tr('Показать больше'),
    onclick: async () => {
      try {
        await whole(m);
      } catch {
        return; // the link stays to try again
      }

      node.opened = true;
      node.render();
    },
  }));
}

// whole loads all a message says, and those it carries on, into it.
async function whole(m) {
  if (m.textCut) Object.assign(m, { textCut: false }, await api.chatMessage(m.id), { unread: m.unread });

  return m;
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

// showSelection is openvk's bar over the chat while messages are
// selected: how many (a click unselects them), "Удалить" while all are
// mine and may still be taken back, "Ответить" to one and "Переслать".
function showSelection() {
  const messages = selectedMessages();

  $('selection').hidden = selected.size === 0;
  $('selection-count').textContent = selectedCount(selected.size);
  $('selection-delete').hidden = !(messages.length > 0 && messages.every(canDelete));
  $('selection-reply').hidden = !(replies && messages.length === 1 && !messages[0].deleted && chat.state === 'active');
  $('selection-forward').hidden = !messages.some((m) => !m.deleted);
  // One message at a time is pinned and asked who has read it, as on
  // openvk.
  const one = messages.length === 1 && !messages[0].deleted ? messages[0] : null;
  $('selection-pin').hidden = !(one && chat.state === 'active');
  $('selection-pin').textContent = one && chat.pinned?.id === one.id ? tr('Открепить') : tr('Закрепить');
  // Who has read a message only its sender learns.
  $('selection-readers').hidden = !(one && one.author?.id === me.id);
}

// togglePin pins the message selected, or takes the pin off it.
function togglePin(m) {
  clearSelection();
  change(() => (chat.pinned?.id === m.id ? api.unpinChatMessage(chatID) : api.pinChatMessage(chatID, m.id)));
}

// showReaders is openvk's window "Просмотрели сообщение": who in the chat
// has read the message, with whether they are online.
async function showReaders(m) {
  clearSelection();
  const modal = openModal(tr('Просмотрели сообщение'));
  modal.box.classList.add('message-viewers');
  modal.body.append(el('div', { className: 'message-viewers-loader', textContent: tr('Загрузка...') }));
  modal.actions.append(button(tr('Закрыть'), modal.close));

  try {
    const { readers } = await api.chatMessageReaders(chatID, m.id);
    if (readers.length === 0) {
      modal.body.replaceChildren(el('div', { className: 'message-viewers-empty', textContent: tr('Это сообщение ещё никто не прочитал') }));
      return;
    }

    modal.body.replaceChildren(el('div', { className: 'message-viewers-list-wrap' },
      el('div', { className: 'message-viewers-header', textContent: tr('Прочитали: {0}', readers.length) }),
      el('div', { className: 'message-viewers-list' }, ...readers.map((u) => el('div', { className: 'message-viewer-row' },
        el('a', { className: 'message-viewer-ava-link', href: '/' + u.nickname }, el('img', { className: 'message-viewer-ava', src: avatarURL(u, 'small'), alt: '' })),
        el('div', { className: 'message-viewer-info' },
          el('a', { className: 'message-viewer-name', href: '/' + u.nickname, textContent: fullName(u) }),
          el('span', { className: 'message-viewer-time', textContent: u.online ? tr('Онлайн') : tr('не в сети') })))))));
  } catch (e) {
    modal.body.replaceChildren(el('div', { className: 'message-viewers-error', textContent: errorText(e) }));
  }
}

const selectedMessages = () => [...selected].map((id) => shown.get(id)?.message).filter(Boolean);

// forward carries messages on ("Переслать"), into this chat or another
// conversation.
function forward(messages) {
  openForward({ me, messages, here: { chat }, authorOf: (m) => m.author, onSent: showSent, onDone: clearSelection });
}

// showSent puts a message just sent at the end of the chat; opened around
// an old message, the chat goes to its end first.
function showSent(m) {
  if (gap) {
    location.assign('/im/chat/' + encodeURIComponent(chatID));
    return;
  }

  add([m], false);
  toBottom();
}

// confirmDelete takes my messages back from everyone in the chat.
function confirmDelete(messages) {
  const modal = openModal(messages.length > 1 ? tr('Удаление сообщений') : tr('Удаление сообщения'));
  const error = el('div', { className: 'error', hidden: true });
  const remove = button(tr('Удалить'), async () => {
    hideError(error);
    remove.disabled = true;

    try {
      for (const m of messages) {
        await api.deleteChatMessage(m.id);
        edited(takenBack(m));
      }
      clearSelection();
      modal.close();
    } catch (e) {
      showError(error, e);
      remove.disabled = false;
    }
  });

  modal.body.append(error, messages.length > 1
    ? tr('Вы действительно хотите удалить {0}?', messagesCount(messages.length))
    : tr('Сообщение будет удалено у всех участников беседы.'));
  modal.actions.append(remove, button(tr('Отмена'), modal.close, { secondary: true }));
}

// startEdit puts what the message says and its photos back in the form:
// sending saves the edit.
async function startEdit(m) {
  const text = $('text');

  try {
    await whole(m);
  } catch (e) {
    showError($('error'), e);
    return;
  }

  replies.cancel();
  editing = m;
  editingBase = m.text;
  text.value = m.text;
  autosize(text);
  attachments.set(m.photos ?? []);
  showEditing(m, me, m.text);
  // An edit changes the words and the photos; the documents and the
  // songs stay.
  documents.clear();
  audios.clear();
  videos.clear();
  $('doc').hidden = true;
  $('audio').hidden = true;
  $('video').hidden = true;
  $('send').textContent = tr('Сохранить');
  text.focus();
  text.setSelectionRange(text.value.length, text.value.length);
}

function cancelEdit() {
  if (!editing) return;

  editing = null;
  $('text').value = '';
  autosize($('text'));
  attachments.clear();
  $('editing').hidden = true;
  $('doc').hidden = $('audio').hidden = $('video').hidden = false;
  $('send').textContent = tr('Отправить');
}

// edited shows what a message is after its sender edited it or took it
// back; whether I have read it the event does not tell.
function edited(m) {
  const node = shown.get(m.id);
  if (!node?.render) return;

  node.message = { ...node.message, textCut: false, ...m, unread: node.message.unread, seen: node.message.seen };
  node.render();
  if (editing?.id === m.id && m.deleted) cancelEdit();
  if (replies?.replyingTo()?.id === m.id && m.deleted) replies.cancel();
  // The pinned message shows what it says now, or goes when taken back.
  if (chat.pinned?.id === m.id) refresh();
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

  if (atStart) $('empty').after(...nodes);
  else list.append(...nodes);

  for (const m of fresh) {
    if (m.id > newestID) newestID = m.id;
  }

  $('empty').hidden = shown.size > 0;
  regroup(list, (m) => (m.action ? null : m.author));

  return fresh;
}

// readIncoming marks the chat read up to the newest message of the
// others, once the user can see it.
async function readIncoming(messages) {
  const incoming = messages.filter((m) => m.unread);
  if (incoming.length === 0 || document.hidden) return;

  // The newest by ID: UUIDv7 strings compare as their time.
  const newest = incoming.reduce((a, b) => (b.id > a.id ? b : a));

  try {
    await api.markChatRead(chatID, newest.id);
    closePushes('chat:' + chatID).catch(() => {});
    for (const m of incoming) {
      m.unread = false;
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
    const page = await api.chatMessages(chatID, { cursor: olderCursor });
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

// loadNewer continues a chat opened around an old message towards its
// end; with nothing newer left the chat is whole and live again.
async function loadNewer() {
  if (loadingNewer || !gap) return;
  loadingNewer = true;

  try {
    const page = await api.chatMessages(chatID, { after: newestID });

    if (page.messages.length === 0) gap = false;
    await readIncoming(add(page.messages, false));
  } catch (e) {
    showError($('error'), e);
  } finally {
    loadingNewer = false;
  }
}

// fillNewer loads on while the end of the list is in view: a scroll that
// reached it, or a list too short to scroll at all.
async function fillNewer() {
  while (gap && nearBottom()) {
    const before = newestID;

    await loadNewer();
    if (gap && newestID === before) break; // busy or failed: the next scroll tries again
  }
}

// showFound brings the message the chat was opened around into view and
// marks it.
async function showFound() {
  const node = shown.get(foundID);

  // The words may be said after what a collapsed text shows: it opens.
  if (node?.render && foundQuery && node.querySelector('.im-message-text.collapsed')) {
    await whole(node.message).then(() => {
      node.opened = true;
      node.render();
    }).catch(() => {});
  }

  node?.classList.add('found');
  (node?.querySelector('.found-words') ?? node)?.scrollIntoView({ block: 'center' });
  await fillNewer();
}

// firstPage is the chat around the found message, or its newest messages:
// without one, or when it is not there anymore.
async function firstPage() {
  const asked = early;
  early = null;
  if (!foundID) return asked;

  try {
    const page = await asked;

    gap = true;
    return page;
  } catch (e) {
    if (e.status !== 404 && e.status !== 400) throw e;
  }

  return api.chatMessages(chatID);
}

// showNew adds new messages, keeping the view at the bottom if it was.
// What happened in the chat changes who is in it and what it is called.
async function showNew(messages) {
  const stick = nearBottom();
  // The others' messages come unread: an event does not say so.
  const fresh = add(messages.map((m) => (m.out || m.action ? m : { ...m, unread: true })), false);

  if (fresh.length > 0 && stick) toBottom();
  if (fresh.some((m) => m.action)) await refresh();
  await readIncoming(fresh);
}

// catchUp asks for what came after the newest message shown: by polling
// while events are down, and once after they come back.
async function catchUp() {
  // Opened around an old message, the chat catches up by scrolling.
  if (gap) return;

  try {
    const page = await api.chatMessages(chatID, newestID ? { after: newestID } : {});

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
  onEvent('chat_message', (e) => {
    if (e.chat !== chatID) return;
    if (e.message.author) typists.stopped(e.message.author.id);
    if (!gap) showNew([e.message]);
  });
  onEvent('typing', (e) => {
    if (e.chat === chatID) typists.typing(e.user);
  });
  onEvent('chat_edit', (e) => {
    if (e.chat === chatID) edited(e.message);
  });
  // Someone else read the chat: my messages up to there are seen.
  onEvent('chat_seen', (e) => {
    if (e.chat !== chatID) return;

    for (const [id, node] of shown) {
      if (id > e.upTo || !node.message?.out || node.message.seen) continue;
      node.message.seen = true;
      node.classList.remove('unread');
    }
  });
  // Whatever happened while disconnected.
  onEvent('ready', catchUp);
}

async function send(event) {
  event.preventDefault();
  hideError($('error'));

  const text = $('text');
  const sendButton = $('send');

  if (!editing && !text.value.trim() && attachments.count() === 0 && documents.count() === 0 && audios.count() === 0 && videos.count() === 0) return;

  if (attachments.busy()) {
    showError($('error'), tr('Дождитесь, пока загрузятся фотографии.'));
    return;
  }

  sendButton.disabled = true;

  try {
    if (editing) {
      edited(await api.editChatMessage(editing.id, text.value, { ...photoChanges(editing.photos, attachments.ids()), base: editingBase }));
      cancelEdit();
    } else {
      const m = await api.sendChatMessage(chatID, withVideos(text.value, videos.ids()), attachments.ids(), '', [], documents.ids(), replies.replyingTo()?.id ?? '', audios.ids());

      text.value = '';
      autosize(text);
      replies.cancel();
      typingWatch.sent();
      attachments.clear();
      documents.clear();
      audios.clear();
      videos.clear();
      showSent(m);
    }
  } catch (e) {
    showError($('error'), e);
  } finally {
    sendButton.disabled = false;
    text.focus();
  }
}

// change does what a link of the chat asks and shows the chat as it has
// become; the lines of what happened come with the events or the poll.
async function change(request) {
  hideError($('error'));

  try {
    show(await request());
    await catchUp();
  } catch (e) {
    showError($('error'), e);
  }
}

function confirmLeave() {
  const modal = openModal(tr('Покинуть чат'));

  modal.body.append(tr('Вы действительно хотите покинуть эту беседу?'));
  modal.actions.append(
    button(tr('Покинуть чат'), () => {
      modal.close();
      change(() => api.leaveChat(chatID));
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

// confirmDeleteChat is the creator's "Удалить беседу": the chat goes for
// everyone in it with all its messages.
function confirmDeleteChat() {
  const modal = openModal(tr('Удалить беседу'));
  const error = el('div', { className: 'error', hidden: true });

  modal.body.append(error, tr('Вы действительно хотите удалить эту беседу? Она пропадёт у всех участников вместе со всеми сообщениями, и вернуть её будет нельзя.'));
  modal.actions.append(
    button(tr('Удалить'), async (e) => {
      e.target.disabled = true;
      hideError(error);

      try {
        await api.deleteChat(chatID);
        location.assign('/im');
      } catch (err) {
        showError(error, err);
        e.target.disabled = false;
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

// showPinned is the line of the message pinned above the chat: who wrote
// it and its beginning; a click goes to it, "×" takes it down.
function showPinned() {
  const m = chat.pinned;

  $('pinned').hidden = !m;
  if (!m) return;

  $('pinned').replaceChildren(
    el('div', { className: 'chat-pinned-body', title: tr('Перейти к сообщению'), onclick: () => goTo(m.id) },
      el('span', { className: 'chat-pinned-head', textContent: tr('Закреплённое сообщение') }),
      el('b', { textContent: fullName(m.author) }), ': ', ...smileyNodes(snippet(messagePreview(m), 200))),
    chat.state === 'active' && el('a', {
      className: 'chat-pinned-remove',
      title: tr('Открепить сообщение'),
      textContent: '×',
      onclick: () => change(() => api.unpinChatMessage(chatID)),
    }),
  );
}

// goTo shows a message of the chat: where it stands when it is loaded,
// otherwise the chat opens around it.
function goTo(id) {
  const node = shown.get(id);
  if (!node) {
    location.assign('/im/chat/' + encodeURIComponent(chatID) + '?msg=' + encodeURIComponent(id));
    return;
  }

  showJumped(node);
}

// openPhoto is "Обновить фотографию": a square cut out of a picture, as
// the photo of a page.
function openPhoto() {
  openAvatarDialog({
    chat: true,
    upload: async (blob) => {
      show(await api.uploadChatPhoto(chatID, blob));
      await catchUp();
    },
  });
}

// drawAbout is openvk's "О чате" in place of the messages: the photo, the
// name and how many are in the chat, the actions in rows with openvk's
// icons, and who is in it under "Участники (N)", its creator marked; the
// creator removes the others here and may delete the chat. One who left
// sees the search and the people only.
function drawAbout() {
  const active = chat.state === 'active';
  const owner = chat.ownerId === me.id;
  const error = el('div', { className: 'error', hidden: true });
  const info = contactInfo({
    photo: chatPhotoURL(chat, 'large'),
    name: chatTitle(chat),
    status: chatMembers(chat.memberCount),
    rename: active && (async (title) => {
      show(await api.renameChat(chatID, title));
      await catchUp();
    }),
    actions: [
      active && [tr('Обновить аватар'), openPhoto, 'ava'],
      active && chat.photo && [tr('Удалить аватар'), () => change(() => api.deleteChatPhoto(chatID)), 'ava'],
      [tr('Поиск по сообщениям'), '/im?chat=' + encodeURIComponent(chatID), 'search'],
      owner && active && [tr('Удалить беседу'), confirmDeleteChat, 'clean'],
    ],
  });

  // A member as openvk's "chat-member-item": the photo, the name with
  // "создатель" and "Онлайн", and at the right "Написать сообщение", the
  // star that would make them an administrator and the cross that removes
  // them, both the creator's.
  const member = (u) => {
    const mine = u.id === me.id;
    const manages = owner && active && !mine;

    return el('div', { className: 'chat-member-item' },
      el('div', { className: 'inf' },
        el('a', { href: '/' + u.nickname }, el('img', { className: 'chat-member-ava', src: avatarURL(u, 'small'), alt: '' })),
        el('div', { className: 'chat-member-info' },
          el('a', { className: 'chat-member-name', href: '/' + u.nickname, textContent: fullName(u) }),
          u.id === chat.ownerId && el('span', { className: 'chat-member-badge', textContent: tr('создатель') }),
          u.online && el('span', { className: 'chat-member-online', textContent: tr('Онлайн') }))),
      el('div', { className: 'chat-member-actions' },
        !mine && el('a', { className: 'chat-member-action-btn', href: '/im/' + encodeURIComponent(u.nickname), textContent: tr('Написать сообщение') }),
        manages && el('a', { className: 'chat-member-action-mod', title: tr('Назначить администратором') }, el('span', { className: 'chat-mod-star-icon' })),
        manages && el('a', { className: 'chat-member-action-kick', title: tr('Исключить из беседы'), onclick: () => confirmKick(u, error) }, el('span', { className: 'chats-close-icon' }))));
  };

  $('contact').replaceChildren(el('div', { className: 'peer-side' },
    info,
    error,
    el('div', { className: 'peer-actions-container' },
      el('div', { className: 'peer-members-section' },
        el('div', { className: 'chat-tab-2-header' },
          el('b', { textContent: tr('Участники ({0})', chat.memberCount) }),
          active && el('div', { className: 'chat-header-actions' },
            el('a', { textContent: tr('Добавить участников'), onclick: () => openAddMembers(me, chat, (changed) => { show(changed); catchUp(); }) }),
            el('a', { textContent: tr('Покинуть чат'), onclick: confirmLeave }))),
        el('div', { className: 'chat-members-list' }, ...(chat.members ?? []).map(member)))),
  ));
}

// confirmKick is openvk's "Подтверждение" before the creator removes
// someone from the chat.
function confirmKick(u, error) {
  const modal = openModal(tr('Подтверждение'));

  modal.body.append(tr('Вы уверены, что хотите исключить '), el('b', { textContent: fullName(u) }), tr(' из беседы?'));
  modal.actions.append(
    button(tr('Да'), async () => {
      modal.close();
      hideError(error);

      try {
        show(await api.removeChatMember(chatID, u.nickname));
        await catchUp();
      } catch (e) {
        showError(error, e);
      }
    }),
    button(tr('Нет'), modal.close, { secondary: true }),
  );
}

// openAbout shows "О чате".
function openAbout() {
  drawAbout();
  showContact(true);
}

// show draws what surrounds the messages: the name of the chat, how many
// are in it, what I may do, and the form or why there is none.
function show(now) {
  chat = now;

  const active = chat.state === 'active';

  document.title = tr('{0} | Сообщения', chatTitle(chat));
  $('title').replaceChildren(el('a', { href: '/im', textContent: tr('Сообщения') }), ' » ', chatTitle(chat));
  $('chat-avatar').src = chatPhotoURL(chat, 'small');
  renameTab?.(chatTitle(chat));
  showPinned();
  if (!$('contact').hidden) drawAbout();

  const canWrite = active && me.profileCompleted;

  $('form').hidden = !canWrite;
  $('blocked').hidden = canWrite;
  if (!active) {
    const back = chat.state === 'left'
      ? [' ', el('a', { textContent: tr('Вернуться в чат'), onclick: () => change(() => api.returnToChat(chatID)) })]
      : [];

    $('blocked').replaceChildren(chatStateText(chat.state), ...back);
    cancelEdit();
  } else if (!canWrite) {
    $('blocked').replaceChildren(tr('Чтобы писать сообщения, '), el('a', { href: '/edit', textContent: tr('заполните свою страницу') }), '.');
  }

  // What I may do with my messages depends on whether I am in the chat.
  for (const node of shown.values()) node.render?.();
}

// refresh reads the chat again: who is in it and what it is called.
async function refresh() {
  try {
    show(await api.chat(chatID));
  } catch {
    // Stays as it was; the next line of what happened reads it again.
  }
}

// sendSticker sends a sticker by itself and tells whether it was sent.
async function sendSticker(sticker) {
  hideError($('error'));
  if (editing) cancelEdit();

  try {
    showSent(await api.sendChatSticker(chatID, sticker));
    return true;
  } catch (e) {
    showError($('error'), e);
    return false;
  }
}

function setUpInput() {
  $('my-avatar').src = avatarURL(me, 'small');
  attachments = initAttachments({ link: $('attach'), also: [$('graffiti')], input: $('file'), list: $('photos'), error: $('error'), onChange: () => {} });
  initGraffiti({ link: $('graffiti'), attachments });
  documents = initDocuments({ link: $('doc'), list: $('documents'), upload: true });
  audios = initAudios({ link: $('audio'), list: $('audios') });
  videos = initVideos({ link: $('video'), list: $('videos') });
  const showStickers = initStickers({ panel: $('stickers'), error: $('error'), onPick: sendSticker, onSent: () => closeEmoji() });
  const closeEmoji = initEmoji({ link: $('smileys-link'), picker: $('smileys'), input: $('text'), bar: $('recent-smiles'), stickersLink: $('stickers-link'), stickersPanel: $('stickers'), showStickers, stickersEntry: $('stickers-entry') });
  $('form').addEventListener('submit', send);
  keepFocus($('send'));
  $('editing-cancel').addEventListener('click', cancelEdit);
  replies = replyBar({ nodeOf: (id) => shown.get(id), open: goTo });
  // The messages shown already get "Ответить".
  for (const node of shown.values()) node.render?.();
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
  // Editing a message is not writing one.
  typingWatch = watchTyping($('text'), () => (editing ? Promise.resolve() : api.chatTyping(chatID)));
}

// The page is asked for with who is signed in, in one round trip.
const wanted = started(api.chat(chatID));
// The messages first shown: the newest, or those around the one found.
let early = started(api.chatMessages(chatID, foundID ? { around: foundID } : {}));
me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const [found, page] = await Promise.all([wanted, firstPage()]);

    renameTab = peerTabs({ key: 'chat:' + chatID, title: chatTitle(found), href: '/im/chat/' + encodeURIComponent(chatID) });
    fitHeight();
    setUpInput();
    $('selection-count').addEventListener('click', clearSelection);
    $('selection-forward').addEventListener('click', () => forward(selectedMessages()));
    $('selection-delete').addEventListener('click', () => confirmDelete(selectedMessages()));
    $('selection-pin').addEventListener('click', () => togglePin(selectedMessages()[0]));
    $('selection-readers').addEventListener('click', () => showReaders(selectedMessages()[0]));
    $('selection-reply').addEventListener('click', () => {
      const [m] = selectedMessages();
      clearSelection();
      answer(m);
    });
    // As on openvk, the photo of the chat opens "О чате".
    $('chat-avatar').addEventListener('click', openAbout);
    $('contact-link').addEventListener('click', openAbout);
    $('tab-contact').addEventListener('click', openAbout);
    $('tab-dialog').addEventListener('click', () => showContact(false));
    $('messenger').hidden = false;
    show(found);
    // A chat is opened to write in it, as messengers do.
    if (!$('form').hidden) whenShown().then(() => $('text').focus({ preventScroll: true }));
    add(page.messages, false);
    olderCursor = page.nextCursor;
    // At the end, and kept there while the messages take their height.
    const stick = stickToBottom(list);
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

    await readIncoming(page.messages);
    listenToEvents();
    setTimeout(poll, POLL_MS);
  } catch (e) {
    const missing = e.status === 404 || e.status === 400;

    $('title').textContent = missing ? tr('Страница не найдена') : tr('Ошибка');
    showError($('error'), missing ? tr('Такой беседы нет.') : e);
  }
}
