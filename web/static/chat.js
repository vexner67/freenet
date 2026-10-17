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
// (replies.js).

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { initGraffiti } from './graffiti.js';
import { initDocuments } from './docs.js';
import { openAddMembers } from './chats.js';
import { isLive, onEvent } from './events.js';
import { openAvatarDialog } from './avatar.js';
import { attachedNodes, forwardedNode, openForward, takenBack } from './forward.js';
import { CHAT_TITLE_MAX, chatActionParts, chatMembers, chatStateText, chatTitle, formatPostDate, isLongText, messagePreview, photoChanges, selectedCount, snippet } from './labels.js';
import { $, avatarURL, chatPhotoURL, el, fullName, mentionNodes, renderChrome, showCounter, showError, hideError, autosize } from './layout.js';
import { button, openModal } from './modal.js';
import { closePushes } from './push.js';
import { quoteNode, reactionNodes, reactWith, replyBar } from './replies.js';
import { initStickers } from './stickers.js';
import { typingLine, watchTyping } from './typing.js';

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
// typingWatch tells the others while the user types.
let typingWatch;
let editing = null;
// replies runs "Ответить" once the form is there.
let replies = null;
// What the message said when its edit began: saving names it, so an edit
// made in another window meanwhile is not overwritten.
let editingBase = '';
// Message ID → node, so polling does not repeat a message.
const shown = new Map();
// The messages selected by a click, by ID.
const selected = new Set();

const personLink = (user, text = fullName(user)) => el('a', { href: '/' + user.nickname, textContent: text });

// actionNode is a line of what happened in the chat: "Анна пригласила
// Петра Петрова".
function actionNode(m) {
  const [before, whom, after] = chatActionParts(m);

  return el('div', { className: 'chat-action' },
    personLink(m.author), ' ', before, whom && personLink(m.target, whom), after,
    el('span', { className: 'chat-action-date', textContent: formatPostDate(m.createdAt) }),
  );
}

function messageNode(m) {
  if (m.action) return actionNode(m);

  const content = el('div', { className: 'im-message-body' });
  const date = el('div', { className: 'im-message-date' });
  const node = el('div', { className: m.unread ? 'im-message unread' : 'im-message' },
    el('a', { href: '/' + m.author.nickname }, el('img', { src: avatarURL(m.author, 'small'), alt: '' })),
    el('div', { className: 'im-message-content' }, el('a', { className: 'post-name', href: '/' + m.author.nickname, textContent: fullName(m.author) }), content),
    date,
  );

  // render shows what the message is now: an edit changes what it says,
  // and taken back it says nothing.
  node.render = () => {
    const now = node.message;
    const active = chat.state === 'active';
    const mine = now.out && !now.deleted && active;
    const pinned = chat.pinned?.id === now.id;

    node.classList.toggle('deleted', now.deleted);
    content.replaceChildren(...[
      !now.deleted && now.replyTo && quoteNode(now.replyTo, now.replyTo.text, goTo),
      now.deleted ? el('div', { className: 'im-message-text', textContent: 'Сообщение удалено' }) : now.text && textOf(now.text, node),
      ...attachedNodes(now),
      forwardedNode(now, (carried) => textOf(carried, node)),
      ...(now.deleted ? [] : reactionNodes(now, me.id, node.picking, react)),
    ].filter(Boolean));
    date.replaceChildren(...[
      formatPostDate(now.createdAt),
      now.editedAt && !now.deleted && el('span', { className: 'post-edited', title: 'Отредактировано', textContent: ' (ред.)' }),
      replies && !now.deleted && active && el('a', { className: 'im-message-action', textContent: 'Ответить', onclick: () => answer(now) }),
      !now.deleted && active && el('a', {
        className: 'im-message-action',
        textContent: 'Реакция',
        onclick: () => {
          node.picking = !node.picking;
          node.render();
        },
      }),
      mine && !now.sticker && now.editableUntil && new Date(now.editableUntil) > new Date()
        && el('a', { className: 'im-message-action', textContent: 'Редактировать', onclick: () => startEdit(now) }),
      !now.deleted && el('a', { className: 'im-message-action', textContent: 'Переслать', onclick: () => forward([now]) }),
      !now.deleted && active && el('a', {
        className: 'im-message-action',
        textContent: pinned ? 'Открепить' : 'Закрепить',
        onclick: () => change(() => (pinned ? api.unpinChatMessage(chatID) : api.pinChatMessage(chatID, now.id))),
      }),
      mine && now.deletableUntil && new Date(now.deletableUntil) > new Date()
        && el('a', { className: 'im-message-action', textContent: 'Удалить', onclick: () => confirmDelete(now) }),
    ].filter(Boolean));
  };
  node.message = m;
  node.render();

  // A click on the message itself, not on a link or a photo in it,
  // selects it, as in a dialog.
  node.addEventListener('click', (event) => {
    if (event.target.closest('a, img, button')) return;
    if (window.getSelection()?.toString()) return;

    toggleSelected(m.id);
  });

  return node;
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
  const body = el('div', { className: 'im-message-text' }, ...mentionNodes(text, marked ? foundQuery : ''));
  const cut = m.textCut && [...text].length >= CUT_LENGTH;
  if (node.opened || !(cut || isLongText(text))) return body;

  body.classList.add('collapsed');

  return el('div', {}, body, el('a', {
    className: 'post-text-more',
    textContent: 'Показать больше',
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

// showSelection is the bar over the chat while messages are selected.
function showSelection() {
  $('selection').hidden = selected.size === 0;
  $('selection-count').textContent = selectedCount(selected.size);
}

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

function confirmDelete(m) {
  const modal = openModal('Удаление сообщения');
  const error = el('div', { className: 'error', hidden: true });
  const remove = button('Удалить', async () => {
    hideError(error);
    remove.disabled = true;

    try {
      await api.deleteChatMessage(m.id);
      edited(takenBack(m));
      modal.close();
    } catch (e) {
      showError(error, e);
      remove.disabled = false;
    }
  });

  modal.body.append(error, 'Сообщение будет удалено у всех участников беседы.');
  modal.actions.append(remove, button('Отмена', modal.close, { secondary: true }));
}

// startEdit puts what the message says and its photos back in the form:
// sending saves the edit. A message that shares a photo of an album takes
// no photos of its own.
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
  $('editing').hidden = false;
  if (m.sharedPhoto) $('attach').hidden = $('graffiti').hidden = true;
  // An edit changes the words and the photos; the documents stay.
  documents.clear();
  $('doc').hidden = true;
  $('send').textContent = 'Сохранить';
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
  $('doc').hidden = false;
  $('send').textContent = 'Отправить';
}

// edited shows what a message is after its sender edited it or took it
// back; whether I have read it the event does not tell.
function edited(m) {
  const node = shown.get(m.id);
  if (!node?.render) return;

  node.message = { ...node.message, textCut: false, ...m, unread: node.message.unread };
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
  if (foundID) {
    try {
      const page = await api.chatMessages(chatID, { around: foundID });

      gap = true;
      return page;
    } catch (e) {
      if (e.status !== 404 && e.status !== 400) throw e;
    }
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
  // Whatever happened while disconnected.
  onEvent('ready', catchUp);
}

async function send(event) {
  event.preventDefault();
  hideError($('error'));

  const text = $('text');
  const sendButton = $('send');

  if (!editing && !text.value.trim() && attachments.count() === 0 && documents.count() === 0) return;

  if (attachments.busy()) {
    showError($('error'), 'Дождитесь, пока загрузятся фотографии.');
    return;
  }

  sendButton.disabled = true;

  try {
    if (editing) {
      edited(await api.editChatMessage(editing.id, text.value, { ...photoChanges(editing.photos, attachments.ids()), base: editingBase }));
      cancelEdit();
    } else {
      const m = await api.sendChatMessage(chatID, text.value, attachments.ids(), '', [], '', documents.ids(), replies.replyingTo()?.id ?? '');

      text.value = '';
      autosize(text);
      replies.cancel();
      typingWatch.sent();
      attachments.clear();
      documents.clear();
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

function openRename() {
  const modal = openModal('Обновить название', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const title = el('input', { type: 'text', maxLength: CHAT_TITLE_MAX, value: chat.title, placeholder: 'Введите название беседы', ariaLabel: 'Название беседы' });
  const save = button('Сохранить', null, { type: 'submit' });

  modal.body.append(error, title);
  modal.actions.append(save, button('Отмена', modal.close, { secondary: true }));
  title.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    save.disabled = true;

    try {
      show(await api.renameChat(chatID, title.value.trim()));
      modal.close();
      await catchUp();
    } catch (e) {
      showError(error, e);
      save.disabled = false;
    }
  });
}

function confirmLeave() {
  const modal = openModal('Покинуть чат');

  modal.body.append('Вы действительно хотите покинуть эту беседу?');
  modal.actions.append(
    button('Покинуть чат', () => {
      modal.close();
      change(() => api.leaveChat(chatID));
    }),
    button('Отмена', modal.close, { secondary: true }),
  );
}

// confirmDelete is the creator's "Удалить беседу": the chat goes for
// everyone in it with all its messages.
function confirmDelete() {
  const modal = openModal('Удалить беседу');
  const error = el('div', { className: 'error', hidden: true });

  modal.body.append(error, 'Вы действительно хотите удалить эту беседу? Она пропадёт у всех участников вместе со всеми сообщениями, и вернуть её будет нельзя.');
  modal.actions.append(
    button('Удалить', async (e) => {
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
    button('Отмена', modal.close, { secondary: true }),
  );
}

// showPinned is the line of the message pinned above the chat: who wrote
// it and its beginning; a click goes to it, "×" takes it down.
function showPinned() {
  const m = chat.pinned;

  $('pinned').hidden = !m;
  if (!m) return;

  $('pinned').replaceChildren(
    el('div', { className: 'chat-pinned-body', title: 'Перейти к сообщению', onclick: () => goTo(m.id) },
      el('span', { className: 'chat-pinned-head', textContent: 'Закреплённое сообщение' }),
      el('b', { textContent: fullName(m.author) }), ': ', snippet(messagePreview(m), 200)),
    chat.state === 'active' && el('a', {
      className: 'chat-pinned-remove',
      title: 'Открепить сообщение',
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

  for (const other of list.querySelectorAll('.im-message.found')) other.classList.remove('found');
  node.classList.add('found');
  node.scrollIntoView({ block: 'center' });
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

// openAbout is "О чате": its photo, who is in it, its creator marked;
// anyone in it changes the photo, the creator removes the others here.
function openAbout() {
  const modal = openModal('О чате');
  const error = el('div', { className: 'error', hidden: true });
  const people = el('div', { className: 'chat-people tall' });

  function fill() {
    people.replaceChildren(...(chat.members ?? []).map((u) => el('div', { className: 'chat-person' },
      el('img', { src: avatarURL(u, 'small'), alt: '' }),
      el('span', {},
        personLink(u),
        u.id === chat.ownerId && el('span', { className: 'hint', textContent: ' — создатель' }),
        chat.ownerId === me.id && u.id !== me.id && chat.state === 'active' && el('a', {
          className: 'chat-kick',
          textContent: 'Исключить из беседы',
          onclick: async () => {
            hideError(error);

            try {
              show(await api.removeChatMember(chatID, u.nickname));
              fill();
              await catchUp();
            } catch (e) {
              showError(error, e);
            }
          },
        }),
      ),
    )));
  }

  const photoLinks = el('div', { className: 'chat-about-links' },
    el('a', { textContent: 'Обновить фотографию', onclick: () => { modal.close(); openPhoto(); } }),
    chat.photo && ' | ',
    chat.photo && el('a', { textContent: 'Удалить фотографию', onclick: () => { modal.close(); change(() => api.deleteChatPhoto(chatID)); } }),
  );

  fill();
  modal.box.classList.add('chat-dialog');
  modal.body.append(
    error,
    el('img', { className: 'chat-about-photo', src: chatPhotoURL(chat, 'large'), alt: '' }),
    chat.state === 'active' ? photoLinks : '',
    el('b', { textContent: chatTitle(chat) }),
    el('div', { className: 'hint', textContent: chatMembers(chat.memberCount) }),
    people,
    chat.ownerId === me.id && chat.state === 'active' && el('p', { className: 'album-delete' },
      'Вы также можете ', el('a', { textContent: 'удалить беседу', onclick: () => { modal.close(); confirmDelete(); } }), '.'),
  );
  modal.actions.append(button('Закрыть', modal.close, { secondary: true }));
}

// show draws what surrounds the messages: the name of the chat, how many
// are in it, what I may do, and the form or why there is none.
function show(now) {
  chat = now;

  const active = chat.state === 'active';
  const link = (text, onclick) => el('a', { textContent: text, onclick });
  const search = el('a', { href: '/im?chat=' + encodeURIComponent(chatID), textContent: 'Поиск по переписке' });
  const links = active
    ? [search, link('Добавить участников', () => openAddMembers(me, chat, (changed) => { show(changed); catchUp(); })), link('Обновить название', openRename), link('Покинуть чат', confirmLeave)]
    : [search];

  document.title = `${chatTitle(chat)} | Сообщения`;
  $('title').replaceChildren(el('a', { href: '/im', textContent: 'Мои Сообщения' }), ' » ', chatTitle(chat));
  $('members').textContent = chatMembers(chat.memberCount);
  $('chat-avatar').src = chatPhotoURL(chat, 'small');
  showPinned();
  $('actions').replaceChildren(...links.flatMap((node, i) => (i ? [' | ', node] : [node])));
  $('about').hidden = !active;

  const canWrite = active && me.profileCompleted;

  $('form').hidden = !canWrite;
  $('blocked').hidden = canWrite;
  if (!active) {
    const back = chat.state === 'left' ? [' ', link('Вернуться в чат', () => change(() => api.returnToChat(chatID)))] : [];

    $('blocked').replaceChildren(chatStateText(chat.state), ...back);
    cancelEdit();
  } else if (!canWrite) {
    $('blocked').replaceChildren('Чтобы писать сообщения, ', el('a', { href: '/edit', textContent: 'заполните свою страницу' }), '.');
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

// sendSticker sends a sticker by itself.
async function sendSticker(sticker) {
  hideError($('error'));
  if (editing) cancelEdit();

  try {
    showSent(await api.sendChatSticker(chatID, sticker));
  } catch (e) {
    showError($('error'), e);
  }
}

function setUpInput() {
  $('my-avatar').src = avatarURL(me, 'small');
  attachments = initAttachments({ link: $('attach'), also: [$('graffiti')], input: $('file'), list: $('photos'), error: $('error'), onChange: () => {} });
  initGraffiti({ link: $('graffiti'), attachments });
  documents = initDocuments({ link: $('doc'), list: $('documents') });
  initStickers({ link: $('stickers-link'), panel: $('stickers'), error: $('error'), onPick: sendSticker });
  $('form').addEventListener('submit', send);
  $('editing-cancel').addEventListener('click', cancelEdit);
  replies = replyBar();
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

me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const [found, page] = await Promise.all([api.chat(chatID), firstPage()]);
    const selectedMessages = () => [...selected].map((id) => shown.get(id)?.message).filter(Boolean);

    setUpInput();
    $('selection-forward').addEventListener('click', () => forward(selectedMessages()));
    $('selection-cancel').addEventListener('click', clearSelection);
    $('about').addEventListener('click', openAbout);
    $('messenger').hidden = false;
    show(found);
    add(page.messages, false);
    olderCursor = page.nextCursor;
    toBottom();
    if (gap) await showFound();

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

    $('title').textContent = missing ? 'Страница не найдена' : 'Ошибка';
    showError($('error'), missing ? 'Такой беседы нет.' : e);
  }
}
