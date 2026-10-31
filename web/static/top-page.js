// "Рейтинги" (/top): the boards of the site, a tab each — the richest
// pages, the miners, the businessmen and the players of the last 7 days
// and the pets with the highest rating of fights. The first three places
// wear medals, as the boards of the forums of the 2000s.

import { api, currentUser, started } from './api.js';
import { primeNameClass, topBoardName, topBoardText, topBoards, topValue } from './labels.js';
import { $, avatarURL, el, fullName, renderChrome, showError, verifiedMark } from './layout.js';
import { petImage } from './pet.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const board = topBoards.includes(params.get('board')) ? params.get('board') : topBoards[0];
const answered = started(api.top(board));

function showTabs() {
  $('tabs').append(...topBoards.map((b) => el('a', {
    className: b === board ? 'active' : '',
    href: `/top?board=${b}`,
    textContent: topBoardName(b),
  })));
}

// The medals of the first three places: gold, silver and bronze on a
// ribbon, glossy as the icons of the 2000s.
const medals = ['/static/icons/medal-1.svg', '/static/icons/medal-2.svg', '/static/icons/medal-3.svg'];

function placeCell(place) {
  if (place <= medals.length) {
    return el('td', { className: 'top-place top-medal' },
      el('img', { src: medals[place - 1], width: 28, height: 36, alt: String(place), title: tr('{0} место', place) }));
  }

  return el('td', { className: 'top-place' }, el('span', { textContent: String(place) }));
}

// personCell is a person of the board; one with Prime wears the colour of
// their name, as in their posts (their pattern is behind the whole row).
function personCell(entry) {
  const path = '/' + entry.user.nickname;

  return el('td', {},
    el('div', { className: 'top-person' },
      el('a', { href: path }, el('img', { className: 'top-avatar', src: avatarURL(entry.user, 'small'), alt: '' })),
      el('div', { className: 'top-who' },
        el('a', { className: ['post-name', primeNameClass(entry.user)].filter(Boolean).join(' '), href: path, textContent: fullName(entry.user) }),
        verifiedMark(entry.user),
        entry.pet && el('div', { className: 'top-pet' },
          petImage(entry.pet.kind, 32),
          el('span', { textContent: entry.pet.name })))));
}

function row(entry) {
  return el('tr', { className: ['top-row', ...(entry.place <= 3 ? ['top-row-medal'] : [])].join(' ') },
    placeCell(entry.place),
    personCell(entry),
    el('td', { className: 'top-value', textContent: topValue(board, entry.value) }));
}

const me = await currentUser().catch(() => null);
renderChrome(me);
showTabs();
document.title = `${topBoardName(board)} | Freenet`;
$('about').textContent = topBoardText(board);

try {
  const { entries } = await answered;
  $('rows').append(...entries.map(row));
  $('table').hidden = entries.length === 0;
  $('empty').hidden = entries.length > 0;
  $('top').hidden = false;
} catch (e) {
  showError($('error'), e);
}
