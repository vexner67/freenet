// What dialogs and chats of several people share: the messages a message
// carries on, shown under its words, and VK's "Переслать" — a window that
// sends messages on into a dialog or a chat.

import { api } from './api.js';
import { chatTitle, conversationOf, conversationPath, conversationValue, formatPostDate, forwardTree, messagePreview, snippet } from './labels.js';
import { el, fullName, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { photosNode, sharedNode, sharedPhotoNode } from './posts.js';

// attachedNodes is what a message shows under its words: its photos, the
// post or the photo of an album it shares.
export function attachedNodes(m) {
  return [
    photosNode(m.photos ?? []),
    m.repost ? sharedNode(m.repost, { canVote: true }) : null,
    m.sharedPhoto ? sharedPhotoNode(m.sharedPhoto) : null,
  ];
}

// forwardedNode shows the messages m carries on, as on VK: under its own
// words, each with who wrote it and when, and under each what it carried
// on itself. textOf draws a text as the page draws those of its messages.
export function forwardedNode(m, textOf) {
  const forwarded = m.forwarded ?? [];
  if (!m.forwardedCount) return null;

  const carried = (trees) => el('div', { className: 'im-forwarded' },
    ...trees.map(({ message: f, carried: under }) => el('div', { className: 'im-forwarded-message' },
      el('a', { className: 'post-name', href: '/' + f.author.nickname, textContent: fullName(f.author) }),
      el('span', { className: 'im-forwarded-date', textContent: formatPostDate(f.createdAt) }),
      f.text ? textOf(f.text) : null,
      ...attachedNodes(f),
      under.length ? carried(under) : null,
    )));

  const shown = carried(forwardTree(forwarded));
  // Those of people whose pages are gone are not shown.
  if (forwarded.length < m.forwardedCount) {
    shown.append(el('div', { className: 'im-forwarded-hint', textContent: 'Пересланное сообщение недоступно.' }));
  }

  return shown;
}

// takenBack is a message after it was deleted for everyone: it says
// nothing.
export function takenBack(m) {
  return {
    ...m, text: '', photos: [], post: '', repost: undefined, albumPhoto: '', sharedPhoto: undefined, forwarded: [], forwardedCount: 0,
    editedAt: null, deletableUntil: '', editableUntil: '', deleted: true,
  };
}

// conversationOption is a dialog or a chat as a choice of where to send.
export function conversationOption(d) {
  return el('option', { value: conversationValue(d), textContent: d.chat ? chatTitle(d.chat) : fullName(d.peer) });
}

// sendTo writes into the dialog or the chat a choice names.
export function sendTo(value, text, { photos = [], post = '', forward = [], albumPhoto = '' } = {}) {
  const { chat, ref } = conversationOf(value);

  return chat
    ? api.sendChatMessage(ref, text, photos, post, forward, albumPhoto)
    : api.sendMessage(ref, text, photos, post, forward, albumPhoto);
}

// openForward carries messages on: which ones, where to ("Кому": this
// conversation, your latest dialogs and chats, or a person found by
// name), your words to go with them, "Отправить" and "Отмена". Messages
// taken back and lines of what happened in a chat have nothing to carry.
// here is the conversation of the page ({ peer } or { chat }), authorOf
// tells who wrote a message; onSent(message) runs when they went here,
// onDone always.
export function openForward({ me, messages, here, authorOf, onSent, onDone = () => {} }) {
  const forward = messages.filter((m) => !m.deleted && !m.action).sort((a, b) => (a.id < b.id ? -1 : 1));
  if (forward.length === 0) return;

  const modal = openModal(forward.length > 1 ? 'Пересылка сообщений' : 'Пересылка сообщения', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const hereValue = conversationValue(here);
  // A chat I am not in anymore takes nothing from me.
  const first = writable(here) ? [conversationOption(here)] : [];
  const chats = el('select', { ariaLabel: 'Переписка' }, ...first);
  const person = el('input', { type: 'search', maxLength: 64, placeholder: 'Найти человека: имя, фамилия или никнейм', ariaLabel: 'Найти человека' });
  const text = el('textarea', { maxLength: 60000, placeholder: '…', ariaLabel: 'Ваш комментарий' });
  const send = button('Отправить', null, { type: 'submit' });

  modal.body.append(
    error,
    el('div', { className: 'forward-list' }, ...forward.map((m) => el('div', {},
      el('b', { textContent: fullName(authorOf(m)) + ': ' }), snippet(messagePreview(m), 80)))),
    el('b', { textContent: 'Кому' }),
    person,
    chats,
    el('b', { textContent: 'Ваш комментарий' }),
    text,
  );
  modal.actions.append(send, button('Отмена', modal.close, { secondary: true }));

  // The latest dialogs and chats, this one first.
  let latest = first;

  api.dialogs().then((page) => {
    latest = [...first, ...page.dialogs.filter((d) => conversationValue(d) !== hereValue && writable(d)).map(conversationOption)];
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

        chats.replaceChildren(...(people.length > 0
          ? people.map((peer) => conversationOption({ peer }))
          : [el('option', { value: '', textContent: 'Никого не нашлось' })]));
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
      if (!to) throw 'Выберите переписку.';

      const sent = await sendTo(to, text.value, { forward: forward.map((m) => m.id) });

      onDone();
      if (to !== hereValue) {
        modal.body.replaceChildren(el('div', {},
          forward.length > 1 ? 'Сообщения пересланы. ' : 'Сообщение переслано. ',
          el('a', { href: conversationPath(to), textContent: 'Перейти' })));
        modal.actions.replaceChildren(button('Закрыть', modal.close));
        return;
      }

      modal.close();
      onSent(sent);
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });

  text.focus();
}

// writable: a chat takes messages only from who is in it.
export function writable(d) {
  return !d.chat || d.chat.state === 'active';
}
