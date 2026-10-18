// Shared page chrome: header links, the sidebar menu and small DOM helpers.
// User data is only ever set with textContent, never as HTML.

import { button, openModal } from './modal.js';
import { accountName, canPark, onAccountChange, park, parkedAccounts, signedInAs, switchTo, tendParked } from './accounts.js';
import { api, idle } from './api.js';
import { ensureKey } from './e2ee.js';
import { connectEvents, onEvent } from './events.js';
import { errorText, foundParts, hashtagPath, isLongText, pushOfferText, pushStateTexts, setTimezone, textParts, unreadCountText } from './labels.js';
import { acceptInvite } from './invite.js';
import { nicknameLookup } from './nicknames.js';
import { closePushes, endPushOffer, enablePush, forgetPushAccount, pushOffer, showUnread, syncPush } from './push.js';

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

// switchAttachMenu is openvk's "Прикрепить" of a form: a link that opens
// the list of what can be attached, each item with its icon (.attach-menu
// of the page). The items are the links the form's script wires, so a
// click on one does what it did, and the list closes after it; a click
// anywhere else and Esc close it too.
function switchAttachMenu(event) {
  const toggle = event.target.closest?.('.attach-toggle');
  const menu = toggle?.closest('.attach-menu');

  for (const open of document.querySelectorAll('.attach-menu.open')) {
    if (open !== menu) open.classList.remove('open');
  }

  menu?.classList.toggle('open');
  if (menu?.classList.contains('open') && menu.classList.contains('attach-auto')) placeAttachList(menu);
}

// placeAttachList opens the list of an .attach-auto menu upwards when
// downwards it would end below what scrolls it (the last messages of a
// dialog), or below the screen.
function placeAttachList(menu) {
  menu.classList.remove('attach-up');

  let box = menu.parentElement;
  while (box && !/(auto|scroll)/.test(getComputedStyle(box).overflowY)) box = box.parentElement;
  const bottom = Math.min(box ? box.getBoundingClientRect().bottom : Infinity, window.innerHeight);

  if (menu.querySelector('.attach-list').getBoundingClientRect().bottom > bottom) menu.classList.add('attach-up');
}

if (typeof document !== 'undefined') {
  document.addEventListener('input', (event) => {
    if (event.target instanceof HTMLTextAreaElement) autosize(event.target);
  });
  document.addEventListener('click', switchAttachMenu);
  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') return;
    for (const open of document.querySelectorAll('.attach-menu.open')) open.classList.remove('open');
  });
}

const knownNickname = nicknameLookup((nicknames) => api.knownNicknames(nicknames).then((r) => r.nicknames));

// mentionNodes are text with the people it names as links to their pages,
// openvk's "[id1|Анна]" and "@ann", its tags ("#поход") as links to
// the search of posts, and its web addresses as links to them. A text says "@word" of anything: the
// link of a word that is nobody's nickname becomes plain text again once
// the server says so, and stays if it cannot be asked. found is what a
// search looked for: the words that say it are highlighted, as on openvk.
export function mentionNodes(text, found = '') {
  return textParts(text).flatMap((p) => {
    if (typeof p === 'string') return foundNodes(p, found);
    if (p.hashtag) return el('a', { href: hashtagPath(p.hashtag) }, ...foundNodes(p.text, found));
    if (p.url) return externalLink(p.url, foundNodes(p.text, found));

    const link = el('a', { href: '/' + encodeURIComponent(p.nickname), textContent: p.text });
    knownNickname(p.nickname).then((known) => {
      if (!known) link.replaceWith(p.text);
    }, () => {});

    return link;
  });
}

// externalLink leads to an address of another site, in a new tab, without
// telling it this page or vouching for it.
export function externalLink(url, children) {
  return el('a', { className: 'text-link', href: url, target: '_blank', rel: 'noopener noreferrer nofollow ugc', title: url }, ...children);
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
  // While the token still works: the pushes of this browser are no longer
  // the account's.
  await forgetPushAccount().catch(() => {});
  await api.logout();
  signedInAs('');
  forgetLook();
  location.href = '/';
}

// switchAccount signs in as an account set aside in this browser, without
// signing out: the account of me waits in its place. The page of signing
// in leads whoever is signed in home.
export async function switchAccount(id, me = null) {
  if (!await switchTo(id, me)) return;

  forgetLook();
  location.href = '/';
}

// addAccount sets the account of me aside and opens the page of signing
// in, for another one ("Добавить аккаунт").
async function addAccount(me) {
  await park(me);
  forgetLook();
  location.href = '/';
}

// accountLinks are the accounts that wait, each a link that switches to
// it: a small photo, the name and how much waits there unread.
export function accountLinks(me = null) {
  return parkedAccounts().map((a) => el('a', {
    className: 'menu-account',
    title: `Войти как ${accountName(a)}`,
    onclick: () => switchAccount(a.id, me),
  },
  a.unread > 0 && el('span', { className: 'counter', textContent: unreadCountText(a.unread) }),
  el('img', { src: a.avatar || '/static/avatar.svg', alt: '' }),
  shortened(accountName(a), ACCOUNT_NAME_LENGTH)));
}

// accountsLink is "Аккаунты" of the left menu. The accounts that wait are
// not in the menu itself: whoever is shown a page of the site would see
// whose else the browser is. The link opens them in a box, each a link
// that switches to it, with "Добавить аккаунт" while there is room for
// one more.
function accountsLink(me) {
  return el('a', { textContent: 'Аккаунты', onclick: () => openAccounts(me) });
}

function openAccounts(me) {
  const modal = openModal('Аккаунты');

  function draw() {
    // Closed meanwhile: there is nothing to draw in.
    if (!modal.box.isConnected) return;

    const waiting = accountLinks(me);

    modal.body.replaceChildren(waiting.length > 0
      ? el('nav', { className: 'menu account-list' }, ...waiting)
      : el('p', { className: 'hint', textContent: 'Других аккаунтов в этом браузере нет.' }));
    modal.actions.replaceChildren(
      canPark() && button('Добавить аккаунт', () => addAccount(me)),
      button('Закрыть', modal.close, { secondary: true }),
    );
  }

  showAccounts(draw);
}

// showAccounts draws the accounts that wait and then looks after them:
// their sessions are kept alive, and what was learnt of them (names,
// photos, unread counters) is drawn again. draw is how the page draws
// them.
export function showAccounts(draw) {
  draw();
  tendParked().then((changed) => changed && draw()).catch(() => {});
}

// Another tab switched accounts or signed out: this page is not its
// account's anymore.
if (typeof window !== 'undefined') onAccountChange(() => location.reload());

// The look of the site is kept in the browser too, so that a page wears
// it before the server has said whose it is; THEME_KEY is a theme chosen
// for this device alone ("Применить стиль только для этого устройства").
// theme.js reads the same keys to put the theme on before the first paint.
const LOOK_KEY = 'freenet.interface';
const THEME_KEY = 'freenet.theme';
// The stylesheets of the themes. A file of /static/ is named in a script
// whole, as one string: the server puts the version of the file into such
// a path, and the browser keeps the file for good.
const THEMES = {
  blue: '/static/themes/blue.css',
  dark: '/static/themes/dark.css',
};

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

  if (Object.hasOwn(THEMES, theme)) {
    const href = THEMES[theme];
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

// theme.js hides the page until it is drawn whole: once the reads it
// makes are answered, the answers that bring more reads included, so it
// does not come up block by block. theme.js shows it anyway after a
// while, for a slow server.
if (typeof document !== 'undefined') idle().then(() => document.documentElement.classList.remove('pending'));

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

  // As openvk's: "ред." at the right of the first link, the links of the
  // pages, and under a line those of what the site keeps for you.
  menu?.replaceChildren(...[
    el('a', { className: 'menu-edit', href: '/edit', textContent: 'ред.', title: 'Редактировать страницу' }),
    el('a', { href: profilePath(me), textContent: 'Страница' }),
    menuLink(me, 'photos', '/albums', 'Фотографии'),
    menuLink(me, 'messages', '/im', 'Сообщения', 'messages-count'),
    menuLink(me, 'notes', '/notes', 'Заметки'),
    menuLink(me, 'groups', '/groups', 'Группы'),
    menuLink(me, 'feed', '/feed', 'Новости'),
    menuLink(me, 'notifications', '/notifications', 'Уведомления', 'notifications-count'),
    el('a', { href: '/settings', textContent: 'Настройки' }),
    el('div', { className: 'menu-divider' }),
    el('a', { href: '/stickers', textContent: 'Стикеры' }),
    el('a', { href: '/docs', textContent: 'Документы' }),
    accountsLink(me),
    el('a', { href: '/search', textContent: 'Поиск' }),
    // The header says "выход"; the phone, whose header is narrow, has it
    // here too, as openvk's.
    el('a', { className: 'menu-phone', href: '/', textContent: 'Выйти', onclick: logout }),
  ].filter(Boolean));
  showCounter('messages');
  showCounter('notifications');
  // The accounts that wait are looked after though the menu does not
  // show them.
  showAccounts(() => {});

  // /me tells the clubs kept in the menu; an answer that is not /me's
  // (saved settings) leaves those the page already knows.
  if (me.pinnedClubs) pinnedClubs = me.pinnedClubs;
  drawPinnedClubs(menu);
}

// renderChrome fills the header and the sidebar for a signed-in user or a guest.
export function renderChrome(me) {
  const links = $('header-links');
  const menu = $('menu');

  addMenuToggle(menu);
  // A guest is not shown what is blurred as adult (posts.js).
  document.body.classList.toggle('guest', !me);
  if (me) signedInAs(me.id);

  if (me?.deactivated) {
    renderDeactivatedChrome(me, links, menu);
    return;
  }

  if (me) {
    links.replaceChildren(
      el('a', { href: profilePath(me), textContent: 'моя страница' }),
      el('a', { href: '/feed', textContent: 'новости' }),
      el('a', { href: '/invite', textContent: 'пригласить', className: 'header-invite' }),
      el('a', { href: '/', textContent: 'выход', onclick: logout }),
    );
    applyInterface(me);
    // The page was made by an invitation link: the inviter is followed.
    acceptInvite(me).catch(() => {});

    // Live counters: the stream says them on connecting and on changes.
    onEvent('ready', applyCounters);
    onEvent('counters', applyCounters);
    connectEvents();
    // The key messages of dialogs are sealed with: made here for one who
    // has none, checked now and then against the server's.
    ensureKey(me.id).catch(() => {});
    // The pushes of this browser, when on, go to whoever is signed in.
    syncPush(me).catch(() => {});
    offerPush(me).catch(() => {});
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
      el('a', { href: '/search', textContent: 'Поиск' }),
    );
  }
}

// offerPush asks once over the page of a phone whether to send pushes
// there, as an app would: the browser asks for permission only on a tap,
// and the switch in the settings is a long way off on a phone. An iPhone
// is told first how to get them at all.
async function offerPush(me) {
  const body = document.querySelector('.page-body');
  if (!body || location.pathname === '/settings') return;

  const offer = await pushOffer();
  if (!offer) return;

  const bar = el('div', { className: 'notice push-offer' });
  const done = () => {
    endPushOffer();
    bar.remove();
  };

  const buttons = offer === 'install'
    ? [button('Понятно', done, { secondary: true })]
    : [
      button('Включить уведомления', async () => {
        done();
        await enablePush(me).catch(() => {});
      }),
      button('Не сейчас', done, { secondary: true }),
    ];

  bar.append(el('p', { textContent: offer === 'install' ? pushStateTexts.install : pushOfferText }),
    el('div', { className: 'push-offer-actions' }, ...buttons));
  body.prepend(bar);
}

// addMenuToggle puts the button that opens the left menu into the header:
// on a phone the menu is not beside the page but under the header, as on
// m.vk.com (style.css hides the button on a wide screen).
function addMenuToggle(menu) {
  const header = document.querySelector('.header');
  if (!menu || !header || header.querySelector('.menu-toggle')) return;

  const toggle = el('a', { className: 'menu-toggle', title: 'Меню', ariaLabel: 'Меню', ariaExpanded: 'false' },
    el('span'), el('span'), el('span'));
  toggle.setAttribute('role', 'button');
  toggle.addEventListener('click', () => {
    const open = document.body.classList.toggle('menu-open');
    toggle.ariaExpanded = String(open);
  });

  header.prepend(toggle);
}

// renderDeactivatedChrome is openvk's for the owner of a deleted page: no
// menu, only "выход" in the header, and no page but their own, where they
// can restore it.
function renderDeactivatedChrome(me, links, menu) {
  links.replaceChildren(el('a', { href: '/', textContent: 'выход', onclick: logout }));

  // Of the menu only "Аккаунты" is left, when
  // there are any: the way to them without signing out.
  const sidebar = menu?.closest('.sidebar');
  const others = parkedAccounts().length > 0;

  if (sidebar) sidebar.hidden = !others;
  const toggle = document.querySelector('.menu-toggle');
  if (toggle) toggle.hidden = !others;
  if (others) {
    menu.replaceChildren(accountsLink(me));
    showAccounts(() => {});
  }

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

  const clubs = el('div', { className: 'menu-clubs' },
    el('div', { className: 'menu-divider' }),
    ...pinnedClubs.map((c) => el('a', { href: pagePath({ ...c, club: true }), textContent: shortened(c.name, PINNED_NAME_LENGTH) })),
  );
  menu.append(clubs);
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
// The names of the accounts that wait are cut the same way.
const ACCOUNT_NAME_LENGTH = 14;

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

// What the counters say, for the icon of the site on the home screen.
const unread = {};

// Pushes about what is read here leave the notification shade.
const pushTags = { messages: ['im:', 'chat:'], notifications: ['n:'] };

export function setCounter(name, n) {
  counterVersions[name]++;

  unread[name] = n;
  if (unread.messages !== undefined && unread.notifications !== undefined) showUnread(unread.messages + unread.notifications);
  if (n <= 0) closePushes(...pushTags[name]).catch(() => {});

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

// chatPhotoURL is the photo of a chat or, without one, the placeholder.
export function chatPhotoURL(chat, size) {
  return chat.photo?.[size] ?? '/static/chat.svg';
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
