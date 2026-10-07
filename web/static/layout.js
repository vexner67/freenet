// Shared page chrome: header links, the sidebar menu and small DOM helpers.
// User data is only ever set with textContent, never as HTML.

import { api } from './api.js';
import { connectEvents, onEvent } from './events.js';
import { errorText, unreadCountText } from './labels.js';

export const $ = (id) => document.getElementById(id);

export function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  Object.assign(node, props);
  node.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
  return node;
}

export function profilePath(user) {
  return user.profileCompleted ? '/' + user.nickname : '/edit';
}

async function logout(event) {
  event.preventDefault();
  await api.logout();
  location.href = '/';
}

// renderChrome fills the header and the sidebar for a signed-in user or a guest.
export function renderChrome(me) {
  const links = $('header-links');
  const menu = $('menu');

  if (me) {
    links.replaceChildren(
      el('a', { href: profilePath(me), textContent: 'моя страница' }),
      el('a', { href: '/feed', textContent: 'новости' }),
      el('a', { href: '/', textContent: 'выход', onclick: logout }),
    );
    menu?.replaceChildren(
      el('a', { href: profilePath(me), textContent: 'Моя Страница' }),
      el('a', { href: '/im', textContent: 'Мои Сообщения' },
        el('span', { className: 'counter', id: 'messages-count', hidden: true })),
      el('a', { href: '/feed', textContent: 'Мои Новости' }),
      el('a', { href: '/notifications', textContent: 'Мои Ответы' },
        el('span', { className: 'counter', id: 'notifications-count', hidden: true })),
      el('a', { href: '/search', textContent: 'Люди' }),
      el('a', { href: '/edit', textContent: 'Редактировать' }),
      el('a', { href: '/', textContent: 'Выйти', onclick: logout }),
    );
    showCounter('messages');
    showCounter('notifications');

    // Live counters: the stream says them on connecting and on changes.
    onEvent('ready', applyCounters);
    onEvent('counters', applyCounters);
    connectEvents();
  } else {
    links.replaceChildren(
      el('a', { href: '/', textContent: 'вход' }),
      el('a', { href: '/', textContent: 'регистрация' }),
    );
    menu?.replaceChildren(
      el('a', { href: '/', textContent: 'Вход' }),
      el('a', { href: '/', textContent: 'Регистрация' }),
      el('a', { href: '/search', textContent: 'Люди' }),
    );
  }
}

// The unread counters of the menu, as on openvk: dialogs with unread
// messages next to "Мои Сообщения", notifications next to "Мои Ответы".
const counters = {
  messages: { id: 'messages-count', count: () => api.unreadDialogs() },
  notifications: { id: 'notifications-count', count: () => api.unreadNotifications() },
};

// A counter's version grows with every change, so an answer that left
// before the change (a page reading everything) does not put the old
// number back.
const counterVersions = { messages: 0, notifications: 0 };

// showCounter fetches a counter. It is a nicety: a failure leaves it empty.
export async function showCounter(name) {
  const version = counterVersions[name];

  try {
    const { count } = await counters[name].count();
    if (version === counterVersions[name]) setCounter(name, count);
  } catch {
    // The page works without it.
  }
}

function applyCounters(event) {
  if (event.messages !== undefined) setCounter('messages', event.messages);
  if (event.notifications !== undefined) setCounter('notifications', event.notifications);
}

export function setCounter(name, n) {
  counterVersions[name]++;

  const counter = $(counters[name].id);
  if (!counter) return;

  counter.textContent = unreadCountText(n);
  counter.hidden = n <= 0;
}

// avatarURL is the photo of a user, author or profile, or the placeholder.
export function avatarURL(user, size) {
  return user.avatar?.[size] ?? '/static/avatar.svg';
}

export function fullName(user) {
  return [user.name, user.surname].filter(Boolean).join(' ');
}

export function showError(container, error) {
  container.textContent = typeof error === 'string' ? error : errorText(error);
  container.hidden = false;
}

export function hideError(container) {
  container.hidden = true;
}
