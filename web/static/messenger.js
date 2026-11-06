// What a dialog and a chat share, as openvk's messenger
// (Web/static/js/messages): the tabs of the conversations opened, over the
// messages; the line of the day over its messages and the messages of one
// author in a row under one head; the window as tall as the screen; the
// icons of a message on hover; and the page "Контакт" ("О чате" of a
// chat) in place of the messages.

import { onEvent } from './events.js';
import { CHAT_TITLE_MAX, messageDay, sameDay } from './labels.js';
import { $, el, isPhone } from './layout.js';
import { tr } from './i18n.js';

const TABS_KEY = 'freenet.imTabs';
// How many conversations the tabs keep: the oldest opened goes first.
const TABS_KEPT = 12;
// Messages of one author closer than this share one head, as on openvk.
const SAME_AUTHOR_MS = 10 * 60_000;

// shownContact: the page "Контакт" stands in place of the messages.
let shownContact = false;

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

  // draw lays the tabs out as openvk's PeerTab: the name, "+N" of what
  // came there and the cross, white on the one open.
  function draw(tabs) {
    bar.replaceChildren(...tabs.map((t) => {
      const mine = t.key === current.key;
      const close = el('span', { className: 'im-peer-tab-close', title: tr('Закрыть') }, el('div', { className: mine ? 'cross white' : 'cross' }));
      const tab = el('div', { className: ['im-peer-tab', mine && 'selected', t.unread > 0 && 'unread'].filter(Boolean).join(' ') },
        el('a', { textContent: t.title, ...(mine ? {} : { href: t.href }) }),
        el('span', { className: 'im-peer-tab-counter', textContent: '+' + (t.unread ?? 0) }),
        close);

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
    bar.hidden = tabs.length === 0 || shownContact;
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
  shownContact = open;
  $('peer-tabs').hidden = open || $('peer-tabs').childElementCount === 0;
  // openvk shows only "К списку друзей" while the page is open.
  $('contact-actions').hidden = open;
  $('contact').hidden = !open;
  $('messenger').hidden = open;
  $('tab-contact').hidden = !open;
  $('tab-contact').classList.toggle('active', open);
  $('tab-dialog').classList.toggle('active', !open);
}

// contactInfo is the head of the page "Контакт", openvk's PeerWindow: the
// photo, the name and what is said under it (online, or how many are in a
// chat), and the actions as rows with openvk's icons, each [text, the
// address or what it does, its icon]. rename, when given, makes the name of
// a chat change in its place, as on openvk.
export function contactInfo({ photo, href, name, status, actions, rename }) {
  const row = ([text, run, icon]) => el('li', { className: `peer-action-${icon}` }, typeof run === 'string'
    ? el('a', { href: run, textContent: text })
    : el('a', { textContent: text, onclick: run }));
  const title = el('div', { className: 'peer-name-1' });

  function showName() {
    const link = rename
      ? el('a', { className: 'peer-link peer-title-editable', title: tr('Изменить название беседы'), textContent: name, onclick: editName })
      : el('a', { className: 'peer-link', href, textContent: name });
    title.replaceChildren(link, status && el('div', { className: 'peer-status' }, el('span', { textContent: status })));
  }

  function editName() {
    const input = el('input', { type: 'text', className: 'peer-title-input', value: name, maxLength: CHAT_TITLE_MAX });
    const save = async () => {
      const value = input.value.trim();
      if (!value || value === name) {
        showName();
        return;
      }
      save.disabled = true;
      await rename(value).catch(() => showName());
    };
    input.addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        save();
      } else if (event.key === 'Escape') {
        event.preventDefault();
        showName();
      }
    });
    title.replaceChildren(el('div', { className: 'peer-title-edit-wrap' },
      input,
      el('button', { className: 'button peer-title-save-btn', type: 'button', textContent: tr('Сохранить'), onclick: save }),
      el('a', { className: 'peer-title-cancel-btn', title: tr('Отмена'), onclick: showName }, el('span', { className: 'chats-close-icon' }))));
    input.focus();
    input.select();
  }

  showName();
  const avatar = el('img', { src: photo, alt: '' });

  return el('div', { className: 'peer-info' },
    el('div', { className: 'peer-avatar' }, href ? el('a', { href }, avatar) : avatar),
    el('div', { className: 'peer-name' },
      title,
      el('div', { className: 'peer-actions-1' }, el('ul', {}, ...actions.filter(Boolean).map(row)))));
}
