// "Рейтинги" (/top): the boards of the site, a tab each — the richest
// pages, the miners, the businessmen and the players of the last 7 days
// and the pets with the highest rating of fights. The first three stand on a
// podium, as the boards of the games of the 2000s; each of the others has a
// bar of how far they are from the leader. The strip over the board says how
// far the viewer is from the place above them. The tabs change without a new
// page, and what is on the screen stays until the next board is here. Every
// board takes the same place: three steps, a pet's line under each name, one
// height of a row, so nothing moves when the tabs change or a number counts.

import { api, currentUser, started } from './api.js';
import { primeNameClass, topBoardName, topBoardText, topBoards, topValue } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showError, hideError, verifiedMark } from './layout.js';
import { gameIcon, helpButton } from './game.js';
import { petImage } from './pet.js';
import { tr } from './i18n.js';

const asked = new URLSearchParams(location.search).get('board');
let board = topBoards.includes(asked) ? asked : topBoards[0];
const answered = started(api.top(board));

const icons = {
  rich: '/static/icons/game/coins.svg',
  miners: '/static/icons/game/pick.svg',
  business: '/static/icons/game/firm.svg',
  casino: '/static/icons/game/cards.svg',
  pets: '/static/icons/game/paw.svg',
};

// Where a board is climbed.
const climbs = {
  miners: ['/mine', () => tr('В шахту')],
  business: ['/business', () => tr('К бизнесу')],
  casino: ['/casino', () => tr('В казино')],
  pets: ['/pet', () => tr('К питомцу')],
};

// The medals of the first three places: gold, silver and bronze on a
// ribbon, glossy as the icons of the 2000s.
const medals = ['/static/icons/medal-1.svg', '/static/icons/medal-2.svg', '/static/icons/medal-3.svg'];

let me = null;
let shown = '';
let question = 0;
const boards = new Map();

function showTabs() {
  $('tabs').replaceChildren(...topBoards.map((b) => el('a', {
    className: b === board ? 'active' : '',
    href: `/top?board=${b}`,
    onclick: (event) => {
      if (event.button || event.metaKey || event.ctrlKey || event.shiftKey) return;
      event.preventDefault();
      open(b, true);
    },
  }, gameIcon(icons[b]), topBoardName(b))));
}

const pagePath = (entry) => '/' + entry.user.nickname;

function who(entry) {
  return [
    el('a', { className: ['post-name', primeNameClass(entry.user)].filter(Boolean).join(' '), href: pagePath(entry), textContent: fullName(entry.user) }),
    verifiedMark(entry.user),
  ];
}

const petOf = (entry, size) => entry.pet && el('div', { className: 'top-pet' }, petImage(entry.pet.kind, size), el('span', { textContent: entry.pet.name }));

// step is a place of the podium; its number counts up.
function step(entry) {
  const sum = el('b', { className: 'top-step-sum' });

  sum.textContent = topValue(board, entry.value);

  return el('div', { className: `top-step top-step-${entry.place}${entry.user.id === me?.id ? ' top-me' : ''}` },
    el('a', { className: 'top-step-face', href: pagePath(entry) },
      el('img', { className: 'top-step-photo', src: avatarURL(entry.user, 'small'), alt: '' }),
      el('img', { className: 'top-step-medal', src: medals[entry.place - 1], width: 22, height: 28, alt: String(entry.place), title: tr('{0} место', entry.place) })),
    el('div', { className: 'top-step-name' }, ...who(entry)),
    el('div', { className: 'top-step-pet' }, petOf(entry, 32)),
    sum,
    el('div', { className: 'top-step-block', textContent: String(entry.place) }));
}

// freeStep is a place of the podium nobody has taken yet.
function freeStep(place) {
  return el('div', { className: `top-step top-step-${place} top-step-free` },
    el('span', { className: 'top-step-face' }),
    el('div', { className: 'top-step-name', textContent: '—' }),
    el('div', { className: 'top-step-pet' }),
    el('b', { className: 'top-step-sum', textContent: '—' }),
    el('div', { className: 'top-step-block', textContent: String(place) }));
}

function showPodium(entries) {
  // Second, first and third, as a podium stands.
  const places = [2, 1, 3].map((place) => {
    const entry = entries[place - 1];

    return entry?.place <= medals.length ? step(entry) : freeStep(place);
  });

  $('podium').replaceChildren(...places);
}

function row(entry, leader) {
  const bar = el('span', { className: 'top-bar-fill' });

  bar.style.transform = `scaleX(${leader > 0 ? Math.max(0, entry.value) / leader : 0})`;

  return el('tr', { className: entry.user.id === me?.id ? 'top-row top-me' : 'top-row', id: `place-${entry.place}` },
    el('td', { className: 'top-place' }, el('span', { textContent: String(entry.place) })),
    el('td', {},
      el('div', { className: 'top-person' },
        el('a', { href: pagePath(entry) }, el('img', { className: 'top-avatar', src: avatarURL(entry.user, 'small'), alt: '' })),
        el('div', { className: 'top-who' }, ...who(entry), petOf(entry, 18)))),
    el('td', { className: 'top-value' },
      el('span', { textContent: topValue(board, entry.value) }),
      el('span', { className: 'top-bar' }, bar)));
}

function hudCell(icon, name, sum, hint, onclick = null) {
  return el(onclick ? 'button' : 'div', { className: 'game-hud-cell', type: onclick ? 'button' : undefined, onclick },
    el('span', { className: 'game-hud-name' }, gameIcon(icon), name),
    sum,
    el('span', { className: 'hint', textContent: hint }));
}

// goTo shows the viewer their place on the board.
function goTo(place) {
  const node = place <= medals.length ? document.querySelector(`.top-step-${place}`) : $(`place-${place}`);

  node?.scrollIntoView({ block: 'center', behavior: 'smooth' });
}

function showHUD(entries) {
  const mine = entries.find((entry) => entry.user.id === me?.id);
  const above = mine && entries.find((entry) => entry.place === mine.place - 1);
  const place = el('b', { className: 'game-hud-sum', textContent: mine ? tr('{0} место', mine.place) : '—' });
  const leader = el('b', { className: 'game-hud-sum' });
  let hint = tr('вас нет в этом рейтинге');

  if (mine) hint = above ? tr('до {0} места: {1}', above.place, topValue(board, above.value - mine.value)) : tr('вы лидер!');
  leader.textContent = topValue(board, entries[0]?.value ?? 0);

  $('hud').replaceChildren(
    hudCell('/static/icons/game/people.svg', tr('Участников'), el('b', { className: 'game-hud-sum', textContent: String(entries.length) }), tr('в рейтинге')),
    hudCell('/static/icons/game/crown.svg', tr('Лидер'), leader, entries[0] ? fullName(entries[0].user) : tr('пока никого')),
    hudCell('/static/icons/game/podium.svg', tr('Ваше место'), place, hint, mine ? () => goTo(mine.place) : null),
  );
}

function show(entries) {
  const leader = Math.max(0, entries[0]?.value ?? 0);
  const rest = entries.filter((entry) => entry.place > medals.length);
  const climb = climbs[board];

  shown = board;
  $('climb').replaceChildren(helpButton(topBoardText(board)));
  if (climb) {
    const [href, text] = climb;
    $('climb').append(el('a', { className: 'button game-go small', href, textContent: text() }));
  }
  showHUD(entries);
  showPodium(entries);
  $('rows').replaceChildren(...rest.map((entry) => row(entry, leader)));
  $('empty').hidden = entries.length > 0;
  $('top').hidden = false;
  $('top').classList.remove('top-loading');
}

// open shows a board; the one on the screen stays until the new one is here.
async function open(next, heard = false) {
  question += 1;
  const mine = question;

  board = next;
  showTabs();
  document.title = `${topBoardName(board)} | Freenet`;
  if (heard) {
    history.replaceState(null, '', `/top?board=${board}`);
  }
  $('top').classList.toggle('top-loading', shown !== board);

  try {
    const entries = boards.get(board) ?? (await (next === board && !shown ? answered : api.top(board))).entries;

    boards.set(next, entries);
    hideError($('error'));
    if (mine === question) show(entries);
  } catch (e) {
    if (mine === question) showError($('error'), e);
  }
}

document.addEventListener('keydown', (event) => {
  if (event.target.closest?.('input, textarea, select') || !['ArrowLeft', 'ArrowRight'].includes(event.key)) return;

  const at = topBoards.indexOf(board) + (event.key === 'ArrowLeft' ? -1 : 1);

  open(topBoards[(at + topBoards.length) % topBoards.length], true);
});

me = await currentUser().catch(() => null);
renderChrome(me);
open(board);
