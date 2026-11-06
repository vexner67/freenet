// What a dialog and a chat share, as openvk's messenger
// (Web/static/js/messages): the tabs of the conversations opened, over the
// messages; the line of the day over its messages and the messages of one
// author in a row under one head; the window as tall as the screen; the
// icons of a message on hover; and the page "Контакт" ("О чате" of a
// chat) in place of the messages.

import { onEvent } from './events.js';
import { messageDay, sameDay } from './labels.js';
import { $, el, isPhone } from './layout.js';
import { tr } from './i18n.js';

const TABS_KEY = 'freenet.imTabs';
// How many conversations the tabs keep: the oldest opened goes first.
const TABS_KEPT = 12;
// Messages of one author closer than this share one head, as on openvk.
const SAME_AUTHOR_MS = 10 * 60_000;

function savedTabs() {
  try {
    const kept = JSON.parse(localStorage.getItem(TABS_KEY) ?? '[]');
    return Array.isArray(kept) ? kept.filter((t) => t && typeof t.key === 'string' && typeof t.href === 'string') : [];
  } catch {
    return [];
  }
}

function saveTabs(tabs) {
  try {
    localStorage.setItem(TABS_KEY, JSON.stringify(tabs));
  } catch {
    // A full storage only forgets the tabs.
  }
}

// lastTab is where "Просмотр диалогов" of the list of dialogs leads: the
// conversation viewed last, '' when none is open.
export function lastTab() {
  return savedTabs().find((t) => t.last)?.href ?? '';
}

// peerTabs keeps this conversation (key, title, href) among the tabs over
// the messages and draws them: the others lead to theirs and count what
// came there since, "×" closes one. It returns rename(title).
export function peerTabs(current) {
  const bar = $('peer-tabs');

  const update = (change) => {
    const tabs = change(savedTabs());
    saveTabs(tabs);
    draw(tabs);
  };

  function draw(tabs) {
    bar.replaceChildren(...tabs.map((t) => {
      const mine = t.key === current.key;
      const close = el('a', { className: 'im-peer-tab-close', title: tr('Закрыть'), textContent: '×' });
      const tab = el(mine ? 'span' : 'a', {
        className: ['im-peer-tab', mine && 'selected', t.unread > 0 && 'unread'].filter(Boolean).join(' '),
        ...(mine ? {} : { href: t.href }),
      }, el('span', { className: 'im-peer-tab-name', textContent: t.title }), t.unread > 0 && el('b', { className: 'im-peer-tab-counter', textContent: '+' + t.unread }), close);

      close.addEventListener('click', (event) => {
        event.preventDefault();
        event.stopPropagation();
        const rest = savedTabs().filter((o) => o.key !== t.key);
        saveTabs(rest);
        if (!mine) {
          draw(rest);
          return;
        }
        location.assign(rest.at(-1)?.href ?? '/im');
      });

      return tab;
    }));
    bar.hidden = tabs.length === 0;
  }

  update((tabs) => {
    const others = tabs.filter((t) => t.key !== current.key).map(({ last, ...t }) => t);
    const at = tabs.findIndex((t) => t.key === current.key);
    const mine = { ...current, unread: 0, last: true };
    // A tab stays where it stood; a new one comes last.
    if (at >= 0) others.splice(Math.min(at, others.length), 0, mine);
    else others.push(mine);
    return others.slice(-TABS_KEPT);
  });

  // What comes to the other conversations counts on their tabs.
  const bump = (key) => update((tabs) => tabs.map((t) => (t.key === key && key !== current.key ? { ...t, unread: (t.unread ?? 0) + 1 } : t)));
  onEvent('message', (e) => {
    if (!e.message.out) bump('im:' + e.peer.nickname);
  });
  onEvent('chat_message', (e) => {
    if (!e.message.out && !e.message.action) bump('chat:' + e.chat);
  });
  // Another window of the site opened or closed a tab.
  addEventListener('storage', (event) => {
    if (event.key === TABS_KEY) draw(savedTabs());
  });

  return (title) => update((tabs) => tabs.map((t) => (t.key === current.key ? { ...t, title } : t)));
}

// regroup lays the messages of list out as openvk does: the day over the
// first message of a day, and a message by the author of the one before,
// within ten minutes, without its own avatar and name (class
// "same-author"). authorOf(m) is who wrote a message, null for a line of
// what happened in a chat.
export function regroup(list, authorOf) {
  for (const day of list.querySelectorAll(':scope > .im-day')) day.remove();

  let before = null;
  for (const node of list.querySelectorAll(':scope > .im-message, :scope > .chat-action')) {
    const m = node.message;
    if (!m) continue;

    if (!before || !sameDay(before.createdAt, m.createdAt)) {
      node.before(el('div', { className: 'im-day' }, el('b', { textContent: messageDay(m.createdAt) })));
    }

    const author = authorOf(m);
    const previous = before && authorOf(before);
    node.classList.toggle('same-author', Boolean(author && previous && author.id === previous.id
      && sameDay(before.createdAt, m.createdAt) && new Date(m.createdAt) - new Date(before.createdAt) < SAME_AUTHOR_MS));
    before = m;
  }
}

// fitHeight makes the window of the messenger reach the bottom of the
// screen, as on openvk: the messages scroll inside it.
export function fitHeight() {
  if (isPhone()) return;

  const body = document.querySelector('.im-page .page-body');
  const fit = () => {
    const top = body.getBoundingClientRect().top + scrollY;
    document.documentElement.style.setProperty('--im-top', Math.round(top) + 'px');
  };

  fit();
  addEventListener('resize', fit);
  // The chrome (menu, counters) may move the page once drawn.
  new ResizeObserver(fit).observe(document.querySelector('.header'));
}

// hoverIcons are the icons over a message on hover, openvk's "actions-2":
// each [title, what it does, its icon: reply, react or edit].
export function hoverIcons(actions) {
  const shown = actions.filter(Boolean);
  if (!shown.length) return null;

  return el('span', { className: 'im-message-icons' }, ...shown.map(([title, run, icon]) => el('a', {
    className: `im-icon im-icon-${icon}`,
    title,
    onclick: (event) => {
      event.stopPropagation();
      run();
    },
  })));
}

// selectOnClick selects a message by a click on it, as openvk does: not on
// a link, a picture or a field in it, and not when words were picked out
// by dragging.
export function selectOnClick(node, select) {
  node.addEventListener('click', (event) => {
    if (event.target.closest('a, img, button, input, textarea, video, audio, .im-reaction, .im-reaction-picker')) return;
    if (window.getSelection()?.toString()) return;
    select();
  });
}

// showContact shows the page "Контакт" (open) or the messages: the global
// tab "Контакт" is there while the page is.
export function showContact(open) {
  $('contact').hidden = !open;
  $('messenger').hidden = open;
  $('tab-contact').hidden = !open;
  $('tab-contact').classList.toggle('active', open);
  $('tab-dialog').classList.toggle('active', !open);
}

// contactInfo is the head of the page "Контакт": the photo, the name and
// what is said under it (online, or how many are in a chat), and the
// actions as rows with openvk's icons, each [text, the address or what
// it does, its icon].
export function contactInfo({ photo, href, name, status, actions }) {
  const row = ([text, run, icon]) => el('li', { className: `peer-action peer-action-${icon}` }, typeof run === 'string'
    ? el('a', { href: run, textContent: text })
    : el('a', { textContent: text, onclick: run }));

  return el('div', { className: 'peer-info' },
    el('div', { className: 'peer-avatar' }, href ? el('a', { href }, el('img', { src: photo, alt: '' })) : el('img', { src: photo, alt: '' })),
    el('div', { className: 'peer-name' },
      el('div', { className: 'peer-name-1' },
        href ? el('a', { className: 'peer-link', href, textContent: name }) : el('b', { className: 'peer-link', textContent: name }),
        status && el('div', { className: 'peer-status', textContent: status })),
      el('ul', { className: 'peer-actions-1' }, ...actions.filter(Boolean).map(row))));
}
