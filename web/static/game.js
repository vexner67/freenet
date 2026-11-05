// The look of the games of money — "Шахта", "Бизнес", "Акции" — and the
// pages of pets: a block with a grey title and its long text behind a "?"
// in that title.

import { el } from './layout.js';
import { popover } from './controls.js';
import { tr } from './i18n.js';

// gameIcon is a picture of static/icons/game: a whole address, as all of them.
export function gameIcon(src, size = 16) {
  return el('img', { className: 'game-icon', src, alt: '', width: size, height: size });
}

// helpButton is a "?" that opens text in a mini window; text may be a
// function, for words that change.
export function helpButton(text) {
  const node = el('button', { type: 'button', className: 'game-help', title: tr('Как это работает'), ariaLabel: tr('Как это работает') },
    gameIcon('/static/icons/game/help.svg', 16));
  node.addEventListener('click', () => {
    const pop = popover(node, tr('Как это работает'));
    if (!pop) return;
    pop.body.append(...[typeof text === 'function' ? text() : text].flat().map((line) => el('div', { className: 'pop-note', textContent: line })));
    pop.place();
  });
  return node;
}

// gameWindow is a block: a grey title with its icon, what help says behind
// a "?" at its right end, and nodes.
export function gameWindow(icon, title, nodes, help = '') {
  return el('div', { className: 'game-window' },
    el('div', { className: 'game-window-title' }, gameIcon(icon, 16), el('span', { textContent: title }), help ? helpButton(help) : null),
    el('div', { className: 'game-window-body' }, ...nodes.filter(Boolean)));
}
