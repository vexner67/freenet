// Shared page chrome: header links, the sidebar menu and small DOM helpers.
// User data is only ever set with textContent, never as HTML.

import { api } from './api.js';
import { errorText } from './labels.js';

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
      el('a', { href: '/feed', textContent: 'Мои Новости' }),
      el('a', { href: '/search', textContent: 'Люди' }),
      el('a', { href: '/edit', textContent: 'Редактировать' }),
      el('a', { href: '/', textContent: 'Выйти', onclick: logout }),
    );
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
