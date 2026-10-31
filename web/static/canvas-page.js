// "Холст" (/canvas): one canvas of 100×100 cells for the whole site. Anyone
// signed in paints cells in one of 64 colors; anyone, a guest too, sees it
// and who painted a cell. The canvas comes anew every ten seconds while the
// page is in sight. The wheel, two fingers and the buttons zoom it,
// dragging moves it; a press that did not move paints.

import { api, currentUser, started } from './api.js';
import { avatarURL, $, el, fullName, profilePath, renderChrome, showError, hideError, verifiedMark } from './layout.js';
import { canvasCellAt, canvasColors, canvasPalette, canvasPicture, formatPostDate, primeNameClass } from './labels.js';
import { tr } from './i18n.js';

const answered = started(api.canvas());
const refreshEvery = 10_000;
const maxZoom = 40;
const buttonZoom = 1.6;
const wheelZoom = 0.002;
// How far a press moves before it is a drag and not a click.
const dragFrom = 6;
const hoverDelay = 120;

let me = null;
let size = 100;
let cells = null;
let color = 3;
let looking = false;
// The cells painted and not answered yet: a refresh keeps them as drawn.
const unanswered = new Map();

const board = $('canvas');
const frame = $('frame');
const tip = $('tip');
const context = board.getContext('2d');

function draw() {
  board.width = size;
  board.height = size;
  context.putImageData(new ImageData(canvasPicture(cells), size, size), 0, 0);
}

function setCell(picture, x, y, value) {
  const bytes = Uint8Array.from(atob(picture), (c) => c.charCodeAt(0));
  bytes[y * size + x] = value;

  return btoa(String.fromCharCode(...bytes));
}

function show(canvas) {
  size = canvas.size;
  cells = canvas.cells;
  for (const { x, y, value } of unanswered.values()) cells = setCell(cells, x, y, value);
  draw();
}

function showStatus() {
  $('status').replaceChildren(...(me
    ? [tr('Выберите цвет и нажмите на клетку.')]
    : [el('a', { href: '/', textContent: tr('Войдите') }), tr(', чтобы рисовать.')]));
}

function showPalette() {
  $('palette').replaceChildren(...canvasPalette().map((i) => {
    const hex = canvasColors[i];
    const swatch = el('button', {
      type: 'button',
      className: i === color ? 'canvas-swatch active' : 'canvas-swatch',
      title: hex,
      onclick: () => {
        color = i;
        showPalette();
      },
    });
    swatch.style.backgroundColor = hex;

    return swatch;
  }));
}

// view is the part of the canvas in the frame: how many times it is
// enlarged and where its corner is, in pixels of the frame.
const view = { zoom: 1, x: 0, y: 0 };

function place() {
  const side = frame.clientWidth;
  const shown = side * view.zoom;

  view.x = Math.min(0, Math.max(side - shown, view.x));
  view.y = Math.min(0, Math.max(side - shown, view.y));
  board.style.width = `${shown}px`;
  board.style.height = `${shown}px`;
  board.style.left = `${view.x}px`;
  board.style.top = `${view.y}px`;
}

// zoomAt enlarges the canvas keeping the point px, py of the frame still.
function zoomAt(px, py, zoom) {
  const next = Math.min(maxZoom, Math.max(1, zoom));

  view.x = px - ((px - view.x) / view.zoom) * next;
  view.y = py - ((py - view.y) / view.zoom) * next;
  view.zoom = next;
  place();
}

function zoomByButton(factor) {
  const middle = frame.clientWidth / 2;

  zoomAt(middle, middle, view.zoom * factor);
}

function hideTip() {
  clearTimeout(hoverTimer);
  hovered = '';
  asked += 1;
  tip.hidden = true;
  tip.classList.remove('pinned');
}

// showTip puts the line beside the pointer, inside the window.
function showTip(nodes, at, pinned) {
  tip.replaceChildren(...nodes);
  tip.classList.toggle('pinned', pinned);
  tip.hidden = false;

  const gap = 14;
  const left = at.x + gap + tip.offsetWidth > innerWidth ? at.x - gap - tip.offsetWidth : at.x + gap;
  const top = at.y + gap + tip.offsetHeight > innerHeight ? at.y - gap - tip.offsetHeight : at.y + gap;

  tip.style.left = `${Math.max(4, left)}px`;
  tip.style.top = `${Math.max(4, top)}px`;
}

// painterLine is the small line of the profile of who painted a cell.
function painterLine(cell) {
  const person = cell.user;
  const date = el('div', { className: 'canvas-tip-date', textContent: formatPostDate(cell.paintedAt) });

  if (!person) return [el('div', {}, tr('страница удалена'), date)];

  return [
    el('img', { src: avatarURL(person, 'small'), alt: '', width: 28, height: 28 }),
    el('div', {},
      el('a', { href: profilePath(person) },
        el('span', { className: primeNameClass(person), textContent: fullName(person) }), verifiedMark(person)),
      date),
  ];
}

// tell says who painted a cell, or that nobody has; only the last question
// is answered, the pointer has gone from the others.
let asked = 0;

async function tell({ x, y }, at, pinned) {
  asked += 1;
  const question = asked;
  let nodes;

  try {
    nodes = painterLine(await api.canvasCell(x, y));
  } catch (e) {
    if (e.status !== 404) throw e;
    nodes = [tr('{0}: её ещё никто не закрашивал.', tr('Клетка {0}, {1}', x + 1, y + 1))];
  }
  if (question === asked) showTip(nodes, at, pinned);
}

// paint colors the cell at once and asks the server; a refusal puts the
// cell back.
async function paint({ x, y }) {
  const before = atob(cells).charCodeAt(y * size + x);
  const key = `${x},${y}`;
  const mark = { x, y, value: color };

  unanswered.set(key, mark);
  cells = setCell(cells, x, y, color);
  draw();

  try {
    await api.paintCanvas(x, y, color);
    hideError($('error'));
  } catch (e) {
    if (unanswered.get(key) === mark) {
      cells = setCell(cells, x, y, before);
      draw();
    }
    showError($('error'), e);
  }
  if (unanswered.get(key) === mark) unanswered.delete(key);
}

function cellOf(event) {
  const box = board.getBoundingClientRect();

  return canvasCellAt(event.clientX - box.left, event.clientY - box.top, box.width, size);
}

function act(event) {
  const cell = cellOf(event);

  if (looking || !me) {
    looking = false;
    $('look').classList.remove('active');
    tell(cell, { x: event.clientX, y: event.clientY }, true).catch((e) => showError($('error'), e));
    return;
  }
  paint(cell).catch((e) => showError($('error'), e));
}

// The pointers on the frame: one drags or clicks, two pinch.
const pointers = new Map();
let press = null;

function pinch() {
  const [a, b] = [...pointers.values()];

  return { gap: Math.hypot(a.x - b.x, a.y - b.y), x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
}

frame.addEventListener('pointerdown', (event) => {
  if (event.button !== 0) return;

  hideTip();
  frame.setPointerCapture(event.pointerId);
  pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
  press = pointers.size === 1
    ? { x: event.clientX, y: event.clientY, moved: false, pinch: null }
    : press && { ...press, moved: true, pinch: pointers.size === 2 ? pinch() : null };
});

frame.addEventListener('pointermove', (event) => {
  const pointer = pointers.get(event.pointerId);

  if (!pointer) {
    if (event.pointerType === 'mouse') hover(event);
    return;
  }

  const moved = { x: event.clientX - pointer.x, y: event.clientY - pointer.y };
  pointer.x = event.clientX;
  pointer.y = event.clientY;
  if (!press) return;

  if (press.pinch && pointers.size === 2) {
    const now = pinch();
    const box = frame.getBoundingClientRect();

    view.x += now.x - press.pinch.x;
    view.y += now.y - press.pinch.y;
    zoomAt(now.x - box.left, now.y - box.top, view.zoom * (now.gap / press.pinch.gap));
    press.pinch = now;
    return;
  }

  press.moved ||= Math.hypot(event.clientX - press.x, event.clientY - press.y) >= dragFrom;
  if (press.moved) {
    frame.classList.add('dragging');
    view.x += moved.x;
    view.y += moved.y;
    place();
  }
});

function release(event, finished) {
  if (!pointers.delete(event.pointerId)) return;

  if (pointers.size === 0) {
    if (finished && press && !press.moved) act(event);
    press = null;
    frame.classList.remove('dragging');
  }
}

frame.addEventListener('pointerup', (event) => release(event, true));
frame.addEventListener('pointercancel', (event) => release(event, false));

frame.addEventListener('wheel', (event) => {
  event.preventDefault();
  hideTip();

  const box = frame.getBoundingClientRect();

  zoomAt(event.clientX - box.left, event.clientY - box.top, view.zoom * Math.exp(-event.deltaY * wheelZoom));
}, { passive: false });

// A mouse says who painted the cell under it; a finger asks by "Кто
// закрасил клетку?".
let hovered = '';
let hoverTimer = 0;

function hover(event) {
  if (tip.classList.contains('pinned')) return;

  const cell = cellOf(event);
  const at = { x: event.clientX, y: event.clientY };
  const key = `${cell.x},${cell.y}`;

  if (key === hovered) {
    if (!tip.hidden) showTip([...tip.childNodes], at, false);
    return;
  }
  hovered = key;
  clearTimeout(hoverTimer);
  hoverTimer = setTimeout(() => tell(cell, at, false).catch(() => {}), hoverDelay);
}

frame.addEventListener('mouseleave', () => {
  if (!tip.classList.contains('pinned')) hideTip();
});

// A pinned line goes with a press anywhere else.
document.addEventListener('pointerdown', (event) => {
  if (tip.classList.contains('pinned') && !tip.contains(event.target)) hideTip();
});

$('look').addEventListener('click', (event) => {
  event.preventDefault();
  looking = !looking;
  $('look').classList.toggle('active', looking);
  $('status').textContent = looking ? tr('Нажмите на клетку, чтобы узнать, кто её закрасил.') : '';
  if (!looking) showStatus();
});

$('zoom-in').addEventListener('click', (event) => {
  event.preventDefault();
  zoomByButton(buttonZoom);
});

$('zoom-out').addEventListener('click', (event) => {
  event.preventDefault();
  zoomByButton(1 / buttonZoom);
});

$('zoom-fit').addEventListener('click', (event) => {
  event.preventDefault();
  view.zoom = 1;
  place();
});

addEventListener('resize', place);

async function refresh() {
  if (document.hidden) return;
  try {
    show(await api.canvas());
  } catch {
    // The next refresh tries again.
  }
}

me = await currentUser().catch(() => null);
renderChrome(me);

try {
  show(await answered);
  showPalette();
  showStatus();
  $('board').hidden = false;
  place();
  setInterval(refresh, refreshEvery);
} catch (e) {
  showError($('error'), e);
}
