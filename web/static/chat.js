// A chat of several people ("беседа") at /im/chat/<id>: messages oldest
// first, older ones load when scrolled to the top, new ones come live
// (events.js) and are polled only while that is down. Between the messages
// stand the lines of what happened: who created the chat, renamed it, was
// added or removed, left or came back. Anyone in the chat writes, adds
// people and renames it; only its creator removes them ("Исключить из
// беседы", in "О чате"). Whoever left ("Покинуть чат") keeps the chat as it
// was and may come back; whoever was removed comes back only when added
// again. Your own message is edited and taken back within a day.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { openAddMembers } from './chats.js';
import { isLive, onEvent } from './events.js';
import { CHAT_TITLE_MAX, chatActionParts, chatMembers, chatStateText, chatTitle, formatPostDate, isLongText } from './labels.js';
import { $, avatarURL, el, fullName, mentionNodes, renderChrome, showCounter, showError, hideError, autosize } from './layout.js';
import { button, openModal } from './modal.js';
import { photosNode } from './posts.js';

const POLL_MS = 5000;
// How close to the top, in pixels, loads older messages.
const LOAD_OLDER_AT = 40;

const chatID = decodeURIComponent(location.pathname.slice('/im/chat/'.length));
const list = $('messages');

let me;
let chat;
let olderCursor = '';
let newestID = '';
let loadingOlder = false;
let attachments;
let editing = null;
// Message ID → node, so polling does not repeat a message.
const shown = new Map();

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
    const mine = now.out && !now.deleted && chat.state === 'active';

    node.classList.toggle('deleted', now.deleted);
    content.replaceChildren(...[
      now.deleted ? el('div', { className: 'im-message-text', textContent: 'Сообщение удалено' }) : now.text && textOf(node),
      photosNode(now.photos ?? []),
    ].filter(Boolean));
    date.replaceChildren(...[
      formatPostDate(now.createdAt),
      now.editedAt && !now.deleted && el('span', { className: 'post-edited', title: 'Отредактировано', textContent: ' (ред.)' }),
      mine && now.editableUntil && new Date(now.editableUntil) > new Date()
        && el('a', { className: 'im-message-action', textContent: 'Редактировать', onclick: () => startEdit(now) }),
      mine && now.deletableUntil && new Date(now.deletableUntil) > new Date()
        && el('a', { className: 'im-message-action', textContent: 'Удалить', onclick: () => confirmDelete(now) }),
    ].filter(Boolean));
  };
  node.message = m;
  node.render();

  return node;
}

// textOf shows what a message says: a long text cut short until "Показать
// больше", which first loads it whole when the chat carried its beginning.
function textOf(node) {
  const m = node.message;
  const body = el('div', { className: 'im-message-text' }, ...mentionNodes(m.text));
  if (node.opened || !(m.textCut || isLongText(m.text))) return body;

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

// whole loads all a message says into it.
async function whole(m) {
  if (m.textCut) Object.assign(m, { textCut: false }, await api.chatMessage(m.id), { unread: m.unread });

  return m;
}

function confirmDelete(m) {
  const modal = openModal('Удаление сообщения');
  const error = el('div', { className: 'error', hidden: true });
  const remove = button('Удалить', async () => {
    hideError(error);
    remove.disabled = true;

    try {
      await api.deleteChatMessage(m.id);
      edited({ ...m, deleted: true, text: '', photos: [] });
      modal.close();
    } catch (e) {
      showError(error, e);
      remove.disabled = false;
    }
  });

  modal.body.append(error, 'Сообщение будет удалено у всех участников беседы.');
  modal.actions.append(remove, button('Отмена', modal.close, { secondary: true }));
}

// startEdit puts what the message says back in the form: sending saves it.
async function startEdit(m) {
  const text = $('text');

  try {
    await whole(m);
  } catch (e) {
    showError($('error'), e);
    return;
  }

  editing = m;
  text.value = m.text;
  autosize(text);
  $('editing').hidden = false;
  $('attach').hidden = true;
  $('photos').hidden = true;
  $('send').textContent = 'Сохранить';
  text.focus();
  text.setSelectionRange(text.value.length, text.value.length);
}

function cancelEdit() {
  if (!editing) return;

  editing = null;
  $('text').value = '';
  autosize($('text'));
  $('editing').hidden = true;
  $('attach').hidden = false;
  $('photos').hidden = attachments.count() === 0;
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
    if (e.chat === chatID) showNew([e.message]);
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

  if (!editing && !text.value.trim() && attachments.count() === 0) return;

  if (!editing && attachments.busy()) {
    showError($('error'), 'Дождитесь, пока загрузятся фотографии.');
    return;
  }

  sendButton.disabled = true;

  try {
    if (editing) {
      edited(await api.editChatMessage(editing.id, text.value));
      cancelEdit();
    } else {
      const m = await api.sendChatMessage(chatID, text.value, attachments.ids());

      text.value = '';
      autosize(text);
      attachments.clear();
      add([m], false);
      toBottom();
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

// openAbout is "О чате": who is in it, its creator marked; the creator
// removes the others here.
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

  fill();
  modal.box.classList.add('chat-dialog');
  modal.body.append(error, el('b', { textContent: chatTitle(chat) }), el('div', { className: 'hint', textContent: chatMembers(chat.memberCount) }), people);
  modal.actions.append(button('Закрыть', modal.close, { secondary: true }));
}

// show draws what surrounds the messages: the name of the chat, how many
// are in it, what I may do, and the form or why there is none.
function show(now) {
  chat = now;

  const active = chat.state === 'active';
  const link = (text, onclick) => el('a', { textContent: text, onclick });
  const links = active
    ? [link('Добавить участников', () => openAddMembers(me, chat, (changed) => { show(changed); catchUp(); })), link('Обновить название', openRename), link('Покинуть чат', confirmLeave)]
    : [];

  document.title = `${chatTitle(chat)} | Сообщения`;
  $('title').replaceChildren(el('a', { href: '/im', textContent: 'Мои Сообщения' }), ' » ', chatTitle(chat));
  $('members').textContent = chatMembers(chat.memberCount);
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

function setUpInput() {
  $('my-avatar').src = avatarURL(me, 'small');
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

me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const [found, page] = await Promise.all([api.chat(chatID), api.chatMessages(chatID)]);

    setUpInput();
    $('about').addEventListener('click', openAbout);
    $('messenger').hidden = false;
    show(found);
    add(page.messages, false);
    olderCursor = page.nextCursor;
    toBottom();

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
    const missing = e.status === 404 || e.status === 400;

    $('title').textContent = missing ? 'Страница не найдена' : 'Ошибка';
    showError($('error'), missing ? 'Такой беседы нет.' : e);
  }
}
