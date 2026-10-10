// Shared page chrome: header links, the sidebar menu and small DOM helpers.
// User data is only ever set with textContent, never as HTML.

import { api } from './api.js';
import { connectEvents, onEvent } from './events.js';
import { errorText, isLongText, unreadCountText } from './labels.js';

export const $ = (id) => document.getElementById(id);

export function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  Object.assign(node, props);
  node.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
  return node;
}

// autosize makes a textarea as tall as what is written in it, as openvk's
// do: never lower than its style sheet makes it, and back to that when
// the text goes. It runs on every input; call it after setting the value
// from a script.
export function autosize(textarea) {
  textarea.style.height = '';
  textarea.style.overflowY = '';

  if (textarea.scrollHeight > textarea.clientHeight) {
    textarea.style.height = textarea.scrollHeight + 5 + 'px';
    textarea.style.overflowY = 'hidden';
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('input', (event) => {
    if (event.target instanceof HTMLTextAreaElement) autosize(event.target);
  });
}

// textNode shows what a post or a comment says. Long text is cut short
// with openvk's "Показать больше", unless whole.
export function textNode(text, { whole = false } = {}) {
  const node = el('div', { className: 'post-text', textContent: text });
  if (whole || !isLongText(text)) return node;

  node.classList.add('collapsed');

  const more = el('a', { className: 'post-text-more', textContent: 'Показать больше' });
  more.addEventListener('click', () => {
    node.classList.remove('collapsed');
    more.remove();
  });

  const wrap = el('div', {}, node, more);

  return wrap;
}

export function profilePath(user) {
  return user.profileCompleted ? '/' + user.nickname : '/edit';
}

// clubRef names a club in its paths: its address or, without one, its ID.
export function clubRef(club) {
  return club.address || club.id;
}

// pagePath is the page of an author: a user's, or a community's.
export function pagePath(author) {
  return author.club ? '/club/' + encodeURIComponent(clubRef(author)) : '/' + author.nickname;
}

// homePath is where the site opens for me: my page, or the news if
// "Главная страница" of the settings says so.
export function homePath(me) {
  return me.interface?.mainPage === 'feed' ? '/feed' : profilePath(me);
}

// menuLink is a link of the left menu, null when "Левое меню" of the
// settings takes item out; counter is the id of its unread counter.
function menuLink(me, item, href, text, counter) {
  if (me.interface?.hiddenMenu?.includes(item)) return null;

  return el('a', { href, textContent: text },
    counter && el('span', { className: 'counter', id: counter, hidden: true }));
}

async function logout(event) {
  event.preventDefault();
  await api.logout();
  location.href = '/';
}

// applyInterface draws what "Интерфейс" of the settings changes: the
// shape of the photos and the left menu with its counters. renderChrome
// calls it; the settings call it again when saved.
export function applyInterface(me) {
  const menu = $('menu');

  // "Отображение аватаров" of the settings: round photos for me alone.
  document.body.classList.toggle('avatars-round', me.interface?.avatarStyle === 'round');

  menu?.replaceChildren(...[
    el('a', { href: profilePath(me), textContent: 'Моя Страница' }),
    menuLink(me, 'photos', '/albums', 'Мои Фотографии'),
    menuLink(me, 'messages', '/im', 'Мои Сообщения', 'messages-count'),
    menuLink(me, 'groups', '/groups', 'Мои Группы'),
    menuLink(me, 'events', '/events', 'Мои Встречи'),
    menuLink(me, 'feed', '/feed', 'Мои Новости'),
    menuLink(me, 'notifications', '/notifications', 'Мои Ответы', 'notifications-count'),
    el('a', { href: '/settings', textContent: 'Мои Настройки' }),
    el('a', { href: '/search', textContent: 'Люди' }),
    el('a', { href: '/', textContent: 'Выйти', onclick: logout }),
  ].filter(Boolean));
  showCounter('messages');
  showCounter('notifications');
  showPinnedClubs(menu);
}

// renderChrome fills the header and the sidebar for a signed-in user or a guest.
export function renderChrome(me) {
  const links = $('header-links');
  const menu = $('menu');

  if (me?.deactivated) {
    renderDeactivatedChrome(me, links, menu);
    return;
  }

  if (me) {
    links.replaceChildren(
      el('a', { href: profilePath(me), textContent: 'моя страница' }),
      el('a', { href: '/feed', textContent: 'новости' }),
      el('a', { href: '/', textContent: 'выход', onclick: logout }),
    );
    applyInterface(me);

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

// renderDeactivatedChrome is openvk's for the owner of a deleted page: no
// menu, only "выход" in the header, and no page but their own, where they
// can restore it.
function renderDeactivatedChrome(me, links, menu) {
  links.replaceChildren(el('a', { href: '/', textContent: 'выход', onclick: logout }));

  const sidebar = menu?.closest('.sidebar');
  if (sidebar) sidebar.hidden = true;

  const own = profilePath(me);
  if (decodeURIComponent(location.pathname) !== own) location.replace(own);
}

// showPinnedClubs adds the clubs you run and keep in the menu under it,
// after a divider, as openvk does ("Добавить в левое меню"). A nicety: a
// failure leaves the menu as it is.
export async function showPinnedClubs(menu) {
  if (!menu) return;

  try {
    const { clubs } = await api.pinnedClubs();

    menu.querySelector('.menu-clubs')?.remove();
    if (clubs.length === 0) return;

    menu.append(el('div', { className: 'menu-clubs' },
      el('div', { className: 'menu-divider' }),
      ...clubs.map((c) => el('a', { href: pagePath({ ...c, club: true }), textContent: shortened(c.name, PINNED_NAME_LENGTH) })),
    ));
  } catch {
    // The page works without it.
  }
}

// openvk cuts the names of pinned clubs to fit the menu.
const PINNED_NAME_LENGTH = 14;

function shortened(s, n) {
  return s.length > n ? s.slice(0, n) + '…' : s;
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

// avatarURL is the photo of a user, author or profile, or the placeholder;
// a community has a placeholder of its own, the camera of openvk.
export function avatarURL(user, size) {
  return user.avatar?.[size] ?? (user.club ? '/static/club.svg' : '/static/avatar.svg');
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
