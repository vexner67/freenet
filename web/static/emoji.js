// The emoji of dialogs and chats, as openvk's (Web/static/js/stickers.js
// and messages/components/common.js): the round face at the right of the
// field opens the picker over it — the emoji by groups, the latest used
// first, with the tabs of the groups and of the stickers at its foot — and
// the emoji used last stand beside "Отправить", a click putting one in
// the message. The emoji is a character of the message, so a sealed one
// carries it as any other.

import { EMOJI_GROUPS } from './emoji-data.js';
import { el, emojiNode, isPhone } from './layout.js';
import { tr } from './i18n.js';

const RECENT_KEY = 'freenet.recentEmoji';
// How many emoji the picker remembers, and how many stand by the button.
const RECENT_KEPT = 100;
const RECENT_SHOWN = 8;
// What stands by the button before any is used, as on openvk.
const FIRST_RECENT = ['😊', '😃', '😉', '😄', '👍', '❤', '🔥', '😂'];

// groupName is the name of a group of the picker, openvk's
// "emoji_group_*".
function groupName(slug) {
  return {
    recent: tr('Недавно использовали'),
    smileys_emotion: tr('Смайлы'),
    people_body: tr('Люди'),
    animals_nature: tr('Природа'),
    food_drink: tr('Еда и напитки'),
    travel_places: tr('Мир'),
    activities: tr('Развлечения'),
    objects: tr('Предметы'),
    symbols: tr('Символы'),
    flags: tr('Флаги'),
  }[slug];
}

function recentEmoji() {
  try {
    const kept = JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]');
    return Array.isArray(kept) ? kept.filter((e) => typeof e === 'string' && e) : [];
  } catch {
    return [];
  }
}

function remember(emoji) {
  const kept = [emoji, ...recentEmoji().filter((e) => e !== emoji)].slice(0, RECENT_KEPT);

  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(kept));
  } catch {
    // A full storage only forgets what was used.
  }
}

// barEmoji are those beside the button: the latest used, made up to eight
// with openvk's own.
function barEmoji() {
  const shown = recentEmoji().slice(0, RECENT_SHOWN);
  for (const e of FIRST_RECENT) {
    if (shown.length >= RECENT_SHOWN) break;
    if (!shown.includes(e)) shown.push(e);
  }

  return shown;
}

// insertEmoji puts emoji in input where its caret is and tells the field
// it changed.
function insertEmoji(input, emoji) {
  input.setRangeText(emoji, input.selectionStart, input.selectionEnd, 'end');
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

// initEmoji makes link open picker over input and bar show the latest
// emoji; the stickers are a tab of the picker: stickersLink shows
// stickersPanel in it, and showStickers draws them. It returns close,
// which the page calls when a sticker is sent.
export function initEmoji({ link, picker, input, bar, stickersLink, stickersPanel, showStickers, stickersEntry = null }) {
  const scroll = picker.querySelector('.emoji-picker-scroll');
  const groups = picker.querySelector('.emoji-picker-groups');
  const nav = picker.querySelector('.emoji-category-nav');
  const mainTab = picker.querySelector('.emoji-main-tab');

  const put = (emoji) => {
    insertEmoji(input, emoji);
    remember(emoji);
    showBar();
    // On a phone the keyboard would cover the picker.
    if (!isPhone()) input.focus();
  };

  function showBar() {
    bar.replaceChildren(...barEmoji().map((emoji) => el('span', {
      className: 'im-recent-smile-btn',
      title: emoji,
      // The field keeps its caret and focus.
      onmousedown: (event) => event.preventDefault(),
      onclick: () => put(emoji),
    }, emojiNode(emoji))));
  }

  const item = (emoji) => el('span', { className: 'emoji-picker-item', title: emoji, onclick: () => put(emoji) }, emojiNode(emoji));

  // draw lays the groups out anew: the latest used come first.
  function draw() {
    const recent = recentEmoji();
    const all = [...(recent.length ? [{ slug: 'recent', emojis: recent }] : []), ...EMOJI_GROUPS];

    groups.replaceChildren(...all.map((g) => tagged(el('div', { className: 'emoji-picker-group' },
      el('div', { className: 'group-title' }, el('b', { textContent: groupName(g.slug) })),
      el('div', { className: 'emoji-picker-group-items' }, ...g.emojis.map(item))), g.slug)));
    nav.replaceChildren(...all.map((g, i) => tagged(el('a', {
      className: i === 0 ? 'emoji-cat-btn active' : 'emoji-cat-btn',
      title: groupName(g.slug),
      onclick: () => {
        showEmoji();
        scroll.scrollTop = [...groups.children].find((n) => n.dataset.group === g.slug).offsetTop - groups.offsetTop;
        mark(g.slug);
      },
    }, g.slug === 'recent' ? el('span', { className: 'emoji-cat-btn-recent' }) : emojiNode(String.fromCodePoint(...codePoints(g.icon)))), g.slug)));
  }

  // mark makes the tab of a group the chosen one.
  function mark(slug) {
    for (const b of nav.children) b.classList.toggle('active', b.dataset.group === slug);
  }

  function showEmoji() {
    groups.hidden = false;
    stickersPanel.hidden = true;
    mainTab.classList.add('active');
    stickersLink.classList.remove('active');
  }

  function open() {
    draw();
    showEmoji();
    scroll.scrollTop = 0;
    picker.hidden = false;
    link.classList.add('active');
  }

  function close() {
    picker.hidden = true;
    link.classList.remove('active');
    stickersEntry?.classList.remove('active');
  }

  // The tab of the group in view is the chosen one.
  scroll.addEventListener('scroll', () => {
    if (groups.hidden) return;
    const top = scroll.scrollTop + groups.offsetTop + 4;
    const current = [...groups.children].filter((g) => g.offsetTop <= top).at(-1) ?? groups.firstElementChild;
    if (current) mark(current.dataset.group);
  });
  link.addEventListener('click', () => (picker.hidden ? open() : close()));
  mainTab.addEventListener('click', showEmoji);
  // The face of the stickers under that of the emoji opens the picker on
  // them; a sticker there is sent at once.
  stickersEntry?.addEventListener('click', () => {
    if (!picker.hidden && !stickersPanel.hidden) {
      close();
      return;
    }
    if (picker.hidden) open();
    link.classList.remove('active');
    stickersEntry.classList.add('active');
    stickersLink.click();
  });
  stickersLink.addEventListener('click', () => {
    groups.hidden = true;
    stickersPanel.hidden = false;
    mainTab.classList.remove('active');
    stickersLink.classList.add('active');
    scroll.scrollTop = 0;
    showStickers();
  });
  // A click elsewhere or Esc closes it, as a menu.
  document.addEventListener('click', (event) => {
    if (!picker.hidden && !picker.contains(event.target) && !link.contains(event.target) && !stickersEntry?.contains(event.target)) close();
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && !picker.hidden) close();
  });

  showBar();
  return close;
}

// tagged marks node as that of a group.
function tagged(node, slug) {
  node.dataset.group = slug;
  return node;
}

// codePoints reads the code of an emoji in the sprites back into its
// characters: UTF-16 units in hex, four digits each.
function codePoints(hex) {
  const units = hex.match(/.{4}/g).map((u) => parseInt(u, 16));
  return [...String.fromCharCode(...units)].map((c) => c.codePointAt(0));
}
