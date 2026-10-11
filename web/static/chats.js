// Chats of several people ("беседы"), what their pages share: the path of
// a chat, the list of people to tick ("Выберите друзей, которых вы хотели
// бы видеть в своей беседе") and the boxes "Новая беседа" and "Добавить
// участников" built on it.

import { api } from './api.js';
import { CHAT_TITLE_MAX, defaultChatTitle, errorText } from './labels.js';
import { avatarURL, el, fullName, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';

const SEARCH_DELAY_MS = 300;

export function chatPath(id) {
  return '/im/chat/' + encodeURIComponent(id);
}

// peoplePicker is a list of people to tick: those who follow me, whom
// their default privacy lets me add, or, with words in the field, whoever
// the search of people finds. Without are nicknames not to offer; picked
// are the people ticked, in the order ticked.
function peoplePicker(me, without = new Set()) {
  const picked = new Map();
  const list = el('div', { className: 'chat-people' });
  const count = el('div', { className: 'hint' });
  const search = el('input', { type: 'search', maxLength: 64, placeholder: 'Найти человека: имя, фамилия или никнейм', ariaLabel: 'Найти человека' });
  let followers = [];
  let timer;

  function showCount() {
    count.textContent = picked.size > 0 ? `Выбрано: ${picked.size}` : '';
  }

  function row(user) {
    const box = el('input', { type: 'checkbox', checked: picked.has(user.nickname) });

    box.addEventListener('change', () => {
      if (box.checked) picked.set(user.nickname, user);
      else picked.delete(user.nickname);
      showCount();
    });

    return el('label', { className: 'chat-person' }, box, el('img', { src: avatarURL(user, 'small'), alt: '' }), el('span', { textContent: fullName(user) }));
  }

  function show(people, nobody) {
    const offered = people.filter((u) => u.nickname && u.nickname !== me.nickname && !without.has(u.nickname) && !u.deactivated);

    list.replaceChildren(...(offered.length > 0 ? offered.map(row) : [el('p', { className: 'hint', textContent: nobody })]));
  }

  const showFollowers = () => show(followers, 'У вас пока нет подписчиков — найдите человека по имени.');

  api.followers(me.nickname, '', 50).then((page) => {
    followers = page.users;
    if (!search.value.trim()) showFollowers();
  }).catch(() => showFollowers());

  search.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      const q = search.value.trim();
      if (!q) {
        showFollowers();
        return;
      }

      try {
        const page = await api.search(q);
        // Only the answer to what the field says now is shown.
        if (search.value.trim() === q) show(page.users, 'Никого не нашлось.');
      } catch {
        show([], 'Никого не нашлось.');
      }
    }, SEARCH_DELAY_MS);
  });

  return { nodes: [search, list, count], picked: () => [...picked.values()] };
}

// openNewChat is "Новая беседа": a title, which may be left out — the chat
// is then named by who is in it — and at least two people.
export function openNewChat(me) {
  const modal = openModal('Новая беседа', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const title = el('input', { type: 'text', maxLength: CHAT_TITLE_MAX, placeholder: 'Введите название беседы', ariaLabel: 'Название беседы' });
  const picker = peoplePicker(me);
  const create = button('Создать беседу', null, { type: 'submit' });

  modal.box.classList.add('chat-dialog');
  modal.body.append(
    error,
    title,
    el('p', { className: 'hint', textContent: 'Выберите друзей, которых вы хотели бы видеть в своей беседе.' }),
    ...picker.nodes,
  );
  modal.actions.append(create, button('Отмена', modal.close, { secondary: true }));
  title.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);

    const people = picker.picked();
    if (people.length < 2) {
      showError(error, errorText({ message: 'a chat needs at least two other people' }));
      return;
    }

    create.disabled = true;

    try {
      const chat = await api.createChat(title.value.trim() || defaultChatTitle([me, ...people]), people.map((u) => u.nickname));

      location.assign(chatPath(chat.id));
    } catch (e) {
      showError(error, e);
      create.disabled = false;
    }
  });
}

// openAddMembers is "Добавить участников" of a chat; done gets the chat as
// it has become.
export function openAddMembers(me, chat, done) {
  const modal = openModal('Добавить участников', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const picker = peoplePicker(me, new Set((chat.members ?? []).map((u) => u.nickname)));
  const add = button('Добавить', null, { type: 'submit' });

  modal.box.classList.add('chat-dialog');
  modal.body.append(error, ...picker.nodes);
  modal.actions.append(add, button('Отмена', modal.close, { secondary: true }));

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);

    const people = picker.picked();
    if (people.length === 0) return;

    add.disabled = true;

    try {
      done(await api.addChatMembers(chat.id, people.map((u) => u.nickname)));
      modal.close();
    } catch (e) {
      showError(error, e);
      add.disabled = false;
    }
  });
}
