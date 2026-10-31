// The look and the feel of the games of money — "Шахта", "Бизнес", "Акции" —
// and the pages of pets: a window with a glossy title, its long text behind
// a "?" in that title, and the small rewards of an action: a number flying
// up from what was pressed, a sum counted up, a flash. Everything that moves
// keeps still under reduced motion: the numbers just change.

import { el } from './layout.js';
import { popover } from './controls.js';
import { tr } from './i18n.js';

const still = matchMedia('(prefers-reduced-motion: reduce)').matches;

// gameIcon is a picture of static/icons/game: a whole address, as all of them.
export function gameIcon(src, size = 16) {
  return el('img', { className: 'game-icon', src, alt: '', width: size, height: size });
}

// helpButton is a "?" that opens text in a mini window.
export function helpButton(text) {
  const node = el('button', { type: 'button', className: 'game-help', title: tr('Как это работает'), ariaLabel: tr('Как это работает') },
    gameIcon('/static/icons/game/help.svg', 16));
  node.addEventListener('click', () => {
    const pop = popover(node, tr('Как это работает'));
    if (!pop) return;
    pop.body.append(...[text].flat().map((line) => el('div', { className: 'pop-note', textContent: line })));
    pop.place();
  });
  return node;
}

// gameWindow is a block: a glossy title with its icon, what help says behind
// a "?" at its right end, and nodes.
export function gameWindow(icon, title, nodes, help = '') {
  return el('div', { className: 'game-window' },
    el('div', { className: 'game-window-title' }, gameIcon(icon, 16), el('span', { textContent: title }), help ? helpButton(help) : null),
    el('div', { className: 'game-window-body' }, ...nodes.filter(Boolean)));
}

// floatText sends text up from the middle of node; tone is good, bad or gold.
export function floatText(node, text, tone = 'good') {
  if (still || !node?.isConnected) return;
  const r = node.getBoundingClientRect();
  const fly = el('div', { className: `game-float ${tone}`, textContent: text });
  fly.style.left = `${r.left + window.scrollX + r.width / 2 + (Math.random() - 0.5) * r.width * 0.4}px`;
  fly.style.top = `${r.top + window.scrollY + r.height / 3}px`;
  document.body.append(fly);
  fly.addEventListener('animationend', () => fly.remove());
}

// flash blinks node once in a tone: good, bad or gold.
export function flash(node, tone = 'good') {
  if (still || !node) return;
  node.classList.remove('game-flash-good', 'game-flash-bad', 'game-flash-gold');
  void node.offsetWidth; // the animation starts over on a new press
  node.classList.add(`game-flash-${tone}`);
}

// countTo runs the number of node from one value to another; text writes
// a value out. A new count takes the place of the one going on.
const counts = new WeakMap();

export function countTo(node, from, to, text, ms = 500) {
  cancelAnimationFrame(counts.get(node));
  if (still || from === to) {
    node.textContent = text(to);
    return;
  }
  const start = performance.now();
  const step = (now) => {
    const part = Math.min(1, (now - start) / ms);
    node.textContent = text(Math.round(from + (to - from) * (1 - (1 - part) ** 3)));
    if (part < 1) counts.set(node, requestAnimationFrame(step));
  };
  counts.set(node, requestAnimationFrame(step));
}
