// Shared page chrome: header links, the sidebar menu and small DOM helpers.
// User data is only ever set with textContent, never as HTML.

import { api } from './api.js';
import { connectEvents, onEvent } from './events.js';
import { errorText, foundParts, isLongText, mentionParts, setTimezone, unreadCountText } from './labels.js';
import { nicknameLookup } from './nicknames.js';

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

const knownNickname = nicknameLookup((nicknames) => api.knownNicknames(nicknames).then((r) => r.nicknames));

// mentionNodes are text with the people it names as links to their pages,
// openvk's "[id1|Анна]" and "@ann". A text says "@word" of anything: the
// link of a word that is nobody's nickname becomes plain text again once
// the server says so, and stays if it cannot be asked. found is what a
// search looked for: the words that say it are highlighted, as on openvk.
export function mentionNodes(text, found = '') {
  return mentionParts(text).flatMap((p) => {
    if (typeof p === 'string') return foundNodes(p, found);

    const link = el('a', { href: '/' + encodeURIComponent(p.nickname), textContent: p.text });
    knownNickname(p.nickname).then((known) => {
      if (!known) link.replaceWith(p.text);
    }, () => {});

    return link;
  });
}

// foundNodes are text with what says query highlighted.
function foundNodes(text, query) {
  if (!query) return [text];

  return foundParts(text, query).map((p) => (p.found ? el('span', { className: 'found-words', textContent: p.text }) : p.text));
}

// textNode shows what a post or a comment says. Long text is cut short
// with openvk's "Показать больше", unless whole. A list carries only the
// beginning of a text of thousands of characters: rest, if given, loads
// all of it when asked to show more. found: the words of a search to
// highlight.
export function textNode(text, { whole = false, rest = null, found = '' } = {}) {
  const node = el('div', { className: 'post-text' }, ...mentionNodes(text, found));
  if (whole || !(rest || isLongText(text))) return node;

  node.classList.add('collapsed');

  const more = el('a', { className: 'post-text-more', textContent: 'Показать больше' });
  more.addEventListener('click', async () => {
    if (rest) {
      try {
        node.replaceChildren(...mentionNodes(await rest(), found));
      } catch {
        // The link stays to try again.
        return;
      }
    }

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
  forgetLook();
  location.href = '/';
}

// The look of the site is kept in the browser too, so that a page wears
// it before the server has said whose it is; THEME_KEY is a theme chosen
// for this device alone ("Применить стиль только для этого устройства").
const LOOK_KEY = 'freenet.interface';
const THEME_KEY = 'freenet.theme';
const THEMES = ['blue', 'dark'];

function stored(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function storedLook() {
  try {
    return JSON.parse(stored(LOOK_KEY)) ?? {};
  } catch {
    return {};
  }
}

function forgetLook() {
  try {
    localStorage.removeItem(LOOK_KEY);
  } catch {
    // The look is a nicety.
  }
}

// deviceTheme is the theme of this device alone, null when it wears the
// one of the settings.
export function deviceTheme() {
  return stored(THEME_KEY);
}

export function setDeviceTheme(theme) {
  try {
    if (theme) localStorage.setItem(THEME_KEY, theme);
    else localStorage.removeItem(THEME_KEY);
  } catch {
    // Without storage the device wears the theme of the settings.
  }
}

// applyLook dresses the page as "Интерфейс" of the settings says: the
// theme, the shape of the photos, the view of the posts, whether adult
// posts are blurred and the gauge of a page shown, and the clock.
function applyLook(ui) {
  const theme = deviceTheme() ?? ui.theme;
  let link = $('theme');

  if (THEMES.includes(theme)) {
    const href = `/static/themes/${theme}.css`;
    if (!link) {
      link = el('link', { id: 'theme', rel: 'stylesheet' });
      document.head.append(link);
    }
    if (link.getAttribute('href') !== href) link.href = href;
  } else {
    link?.remove();
  }

  const { classList } = document.body;
  classList.toggle('avatars-round', ui.avatarStyle === 'round');
  classList.toggle('posts-microblog', ui.postView === 'microblog');
  classList.toggle('nsfw-shown', ui.nsfw === 'show');
  classList.toggle('rating-hidden', Boolean(ui.hideRating));

  setTimezone(ui.timezone);
}

if (typeof document !== 'undefined') applyLook(storedLook());

// isMicroblog: posts are laid out as openvk's "Микроблог".
export function isMicroblog() {
  return document.body.classList.contains('posts-microblog');
}

// adult marks node, a photo that came from a post marked as adult or
// the frame of one: blurred, unless "NSFW-контент" of the settings shows
// such.
export function adult(node, nsfw) {
  node.classList.toggle('photo-nsfw', Boolean(nsfw));

  return node;
}

// revealAdult shows a blurred photo to whoever signed in and clicks it,
// as a click shows a post marked as adult. It tells whether the click was
// spent on that.
export function revealAdult(node) {
  const { classList } = document.body;
  if (!node.classList.contains('photo-nsfw') || classList.contains('nsfw-shown')) return false;

  if (!classList.contains('guest')) node.classList.remove('photo-nsfw');

  return true;
}

// applyInterface draws what "Интерфейс" of the settings changes: the
// look of the page and the left menu with its counters. renderChrome
// calls it; the settings call it again when saved.
export function applyInterface(me) {
  const menu = $('menu');
  const ui = me.interface ?? {};

  try {
    localStorage.setItem(LOOK_KEY, JSON.stringify(ui));
  } catch {
    // The next page asks the server again.
  }
  applyLook(ui);

  menu?.replaceChildren(...[
    el('a', { href: profilePath(me), textContent: 'Моя Страница' }),
    menuLink(me, 'photos', '/albums', 'Мои Фотографии'),
    menuLink(me, 'messages', '/im', 'Мои Сообщения', 'messages-count'),
    menuLink(me, 'notes', '/notes', 'Мои Заметки'),
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

  // /me tells the clubs kept in the menu; an answer that is not /me's
  // (saved settings) leaves those the page already knows.
  if (me.pinnedClubs) pinnedClubs = me.pinnedClubs;
  drawPinnedClubs(menu);
}

// renderChrome fills the header and the sidebar for a signed-in user or a guest.
export function renderChrome(me) {
  const links = $('header-links');
  const menu = $('menu');

  // A guest is not shown what is blurred as adult (posts.js).
  document.body.classList.toggle('guest', !me);

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
    // A guest sees the site as it is, whoever was here before.
    forgetLook();
    applyLook({});

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

// The clubs you run and keep in the left menu ("Добавить в левое меню"),
// as /me told them.
let pinnedClubs = [];

// drawPinnedClubs adds them under the menu, after a divider, as openvk
// does.
function drawPinnedClubs(menu) {
  if (!menu) return;

  menu.querySelector('.menu-clubs')?.remove();
  if (pinnedClubs.length === 0) return;

  menu.append(el('div', { className: 'menu-clubs' },
    el('div', { className: 'menu-divider' }),
    ...pinnedClubs.map((c) => el('a', { href: pagePath({ ...c, club: true }), textContent: shortened(c.name, PINNED_NAME_LENGTH) })),
  ));
}

// showPinnedClubs asks for them again, after one is added or removed. A
// nicety: a failure leaves the menu as it is.
export async function showPinnedClubs(menu) {
  try {
    pinnedClubs = (await api.pinnedClubs()).clubs;
    drawPinnedClubs(menu);
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
