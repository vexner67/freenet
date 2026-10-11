// A dialog at /im/<nickname>, as on openvk: messages oldest first, older
// ones load when scrolled to the top, new ones and read marks come live
// (events.js) and are polled only while that is down. Enter sends,
// Shift+Enter breaks the line. Unread messages are grey either way: the
// peer's until you read them, yours until the peer does. As on VK: any
// message can be deleted for yourself, your own for both for a day
// ("Удалить для всех") and edited for a day; a click on a message selects
// it, and several are deleted or forwarded ("Переслать") at once. A
// message taken back stays as "Сообщение удалено". With ?msg=<id> the
// dialog opens around that message, as a search leads to it.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { isLive, onEvent } from './events.js';
import { errorText, formatPostDate, foundParts, genitiveName, isLongText, messagePreview, selectedCount, snippet } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showCounter, showError, hideError, autosize } from './layout.js';
import { button, openModal } from './modal.js';
import { photosNode, sharedNode, sharedPhotoNode } from './posts.js';

const POLL_MS = 5000;
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
      text && textOf(text, node),
      photosNode(shownMessage.photos ?? []),
      shownMessage.repost ? sharedNode(shownMessage.repost) : null,
      shownMessage.sharedPhoto ? sharedPhotoNode(shownMessage.sharedPhoto) : null,
      forwardedNode(shownMessage, node),
    ].filter(Boolean));
    date.replaceChildren(...[
      formatPostDate(shownMessage.createdAt),
      shownMessage.editedAt && el('span', { className: 'post-edited', title: 'Отредактировано', textContent: ' (ред.)' }),
      canEdit(shownMessage) && el('a', { className: 'im-message-action', textContent: 'Редактировать', onclick: () => startEdit(shownMessage) }),
      !shownMessage.deleted && el('a', { className: 'im-message-action', textContent: 'Переслать', onclick: () => openForward([shownMessage]) }),
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

// CUT_LENGTH is how much of a long text a dialog carries; the message
// says only that some text of it is cut (textCut), not which.
const CUT_LENGTH = 1000;

// textOf shows a text of a message of node, its own or of one it carries
// on: a long one cut short until "Показать больше", as posts are, which
// first loads the message whole when the dialog carried its beginning.
function textOf(text, node) {
  const marked = node.message?.id === foundID && foundQuery;
  const body = marked
    ? el('div', { className: 'im-message-text' },
      ...foundParts(text, foundQuery).map((p) => (p.found ? el('span', { className: 'found-words', textContent: p.text }) : p.text)))
    : el('div', { className: 'im-message-text', textContent: text });
  const cut = node.message?.textCut && [...text].length >= CUT_LENGTH;
  if (node.opened || !(cut || isLongText(text))) return body;

  body.classList.add('collapsed');

  const more = el('a', {
    className: 'post-text-more',
    textContent: 'Показать больше',
    onclick: async () => {
      try {
        await whole(node.message);
      } catch {
        // The link stays to try again.
        return;
      }

      node.opened = true;
      node.render();
    },
  });

  return el('div', {}, body, more);
}

// whole loads all a message says, and those it carries on, into it.
async function whole(m) {
  if (m.textCut) Object.assign(m, { textCut: false }, await api.message(nickname, m.id), { read: m.read });

  return m;
}

// forwardedNode shows the messages a message carries on, as on VK: under
// its own words, each with who wrote it and when.
function forwardedNode(m, node) {
  const forwarded = m.forwarded ?? [];
  if (!m.forwardedCount) return null;

  return el('div', { className: 'im-forwarded' },
    ...forwarded.map((f) => el('div', { className: 'im-forwarded-message' },
      el('a', { className: 'post-name', href: '/' + f.author.nickname, textContent: fullName(f.author) }),
      el('span', { className: 'im-forwarded-date', textContent: formatPostDate(f.createdAt) }),
      f.text ? textOf(f.text, node) : null,
      photosNode(f.photos ?? []),
      f.repost ? sharedNode(f.repost) : null,
      f.sharedPhoto ? sharedPhotoNode(f.sharedPhoto) : null,
    )),
    // Those of people whose pages are gone are not shown.
    forwarded.length < m.forwardedCount ? el('div', { className: 'im-forwarded-hint', textContent: 'Пересланное сообщение недоступно.' }) : null,
  );
}

// canDeleteForAll: your own message, until deletableUntil; the server
// says no once it passes while the page is open.
function canDeleteForAll(m) {
  return m.out && !m.deleted && Boolean(m.deletableUntil) && Date.now() < new Date(m.deletableUntil);
}

// canEdit: your own message, until editableUntil.
function canEdit(m) {
  return m.out && !m.deleted && Boolean(m.editableUntil) && Date.now() < new Date(m.editableUntil);
}

// takenBack is a message after "Удалить для всех": it says nothing.
function takenBack(m) {
  return {
    ...m, text: '', photos: [], post: '', repost: undefined, forwarded: [], forwardedCount: 0,
    editedAt: null, deletableUntil: '', editableUntil: '', deleted: true,
  };
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

// openForward carries messages on into a dialog, as VK's "Переслать":
// which one, your words to go with them, "Отправить" and "Отмена".
// Messages taken back have nothing to carry.
function openForward(messages) {
  const forward = messages.filter((m) => !m.deleted).sort((a, b) => (a.id < b.id ? -1 : 1));
  if (forward.length === 0) return;

  const modal = openModal(forward.length > 1 ? 'Пересылка сообщений' : 'Пересылка сообщения', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const chats = el('select', { ariaLabel: 'Переписка' }, el('option', { value: nickname, textContent: fullName(peer) }));
  const person = el('input', { type: 'search', maxLength: 64, placeholder: 'Найти человека: имя, фамилия или никнейм', ariaLabel: 'Найти человека' });
  const text = el('textarea', { maxLength: 60000, placeholder: '…', ariaLabel: 'Ваш комментарий' });
  const send = button('Отправить', null, { type: 'submit' });

  modal.body.append(
    error,
    el('div', { className: 'forward-list' }, ...forward.map((m) => el('div', {},
      el('b', { textContent: fullName(m.out ? me : peer) + ': ' }), snippet(messagePreview(m), 80)))),
    el('b', { textContent: 'Кому' }),
    person,
    chats,
    el('b', { textContent: 'Ваш комментарий' }),
    text,
  );
  modal.actions.append(send, button('Отмена', modal.close, { secondary: true }));

  const option = (user) => el('option', { value: user.nickname, textContent: fullName(user) });
  // The latest dialogs, this one first.
  let latest = [option(peer)];

  api.dialogs().then((page) => {
    latest = [option(peer), ...page.dialogs.filter((d) => d.peer.nickname !== nickname).map((d) => option(d.peer))];
    if (!person.value.trim()) chats.replaceChildren(...latest);
  }).catch((e) => showError(error, e));

  // Anyone else is found by name, as the search of people finds them;
  // only the answer to what the field says now is shown.
  let timer;

  person.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      const q = person.value.trim();

      hideError(error);
      if (!q) {
        chats.replaceChildren(...latest);
        send.disabled = false;
        return;
      }

      try {
        const page = await api.search(q);
        if (person.value.trim() !== q) return;

        const people = page.users.filter((u) => u.nickname !== me.nickname);

        chats.replaceChildren(...(people.length > 0 ? people.map(option) : [el('option', { value: '', textContent: 'Никого не нашлось' })]));
        send.disabled = people.length === 0;
      } catch (e) {
        showError(error, e);
      }
    }, 300);
  });

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    send.disabled = true;

    try {
      const to = chats.value;
      const sent = await api.sendMessage(to, text.value, [], '', forward.map((m) => m.id));

      clearSelection();
      if (to !== nickname) {
        modal.body.replaceChildren(el('div', {},
          forward.length > 1 ? 'Сообщения пересланы. ' : 'Сообщение переслано. ',
          el('a', { href: '/im/' + encodeURIComponent(to), textContent: 'Перейти' })));
        modal.actions.replaceChildren(button('Закрыть', modal.close));
        return;
      }

      modal.close();
      showSent(sent);
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });

  text.focus();
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
  attachments.set(m.photos ?? []);
  $('editing').hidden = false;
  $('send').textContent = 'Сохранить';
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
  $('editing').hidden = true;
  $('send').textContent = 'Отправить';
}

// edited shows what a message is after its sender, me or the peer,
// edited it or took it back; whether it is read the event does not tell.
function edited(m) {
  const node = shown.get(m.id);
  if (!node) return;

  node.message = { ...node.message, textCut: false, ...m, read: node.message.read };
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

// loadNewer continues a dialog opened around an old message towards its
// end; with nothing newer left the dialog is whole and live again.
async function loadNewer() {
  if (loadingNewer || !gap) return;
  loadingNewer = true;

  try {
    const page = await api.messages(nickname, { after: newestID });

    markTheirRead(page.peerReadUpTo);
    if (page.messages.length === 0) gap = false;
    await readIncoming(add(page.messages, false));
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
    await whole(node.message).then(() => {
      node.opened = true;
      node.render();
    }).catch(() => {});
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
      const page = await api.messages(nickname, { around: foundID });

      gap = true;
      return page;
    } catch (e) {
      if (e.status !== 404 && e.status !== 400) throw e;
    }
  }

  return api.messages(nickname);
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
    if (e.peer.id === peer.id && !gap) showNew([e.message]);
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
    if (attachments.busy()) {
      showError($('error'), 'Дождитесь, пока загрузятся фотографии.');
      return;
    }

    button.disabled = true;

    try {
      edited(await api.editMessage(editing.id, text.value, editing.sharedPhoto ? undefined : attachments.ids()));
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

    text.value = '';
    autosize(text);
    attachments.clear();
    showSent(m);
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
    const page = await firstPage();
    peer = page.peer;

    document.title = `${fullName(peer)} | Сообщения`;
    $('title').replaceChildren(
      el('a', { href: '/im', textContent: 'Мои Сообщения' }),
      ' » ',
      el('a', { href: '/' + peer.nickname, textContent: fullName(peer) }),
    );

    $('search-link').href = '/im?peer=' + encodeURIComponent(peer.nickname);
    $('search-link').hidden = false;
    const selectedMessages = () => [...selected].map((id) => shown.get(id)?.message).filter(Boolean);

    $('selection-forward').addEventListener('click', () => openForward(selectedMessages()));
    $('selection-delete').addEventListener('click', () => confirmDelete(selectedMessages()));
    $('selection-cancel').addEventListener('click', clearSelection);

    $('messenger').hidden = false;
    add(page.messages, false);
    olderCursor = page.nextCursor;
    toBottom();
    await setUpInput(page.canWrite);
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
    $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Ошибка';
    showError($('error'), e.status === 404 ? 'Такого пользователя нет.' : e);
  }
}
