// "Холст" (/canvas): one canvas of 100×100 cells for the whole site. Anyone
// signed in paints a cell a minute in one of 16 colors; anyone, a guest
// too, sees it and who painted a cell. The canvas comes anew every ten
// seconds while the page is in sight.

import { api, currentUser, started } from './api.js';
import { canvasCellAt, canvasColors, canvasPicture, formatPostDate } from './labels.js';
import { $, el, fullName, isPhone, profilePath, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const answered = started(api.canvas());
const zooms = [1, 2, 4];
const refreshEvery = 10_000;

let me = null;
let size = 100;
let cells = null;
let color = 3;
let nextAt = null;
let zoom = 0;
let looking = false;

const board = $('canvas');
const context = board.getContext('2d');

function draw() {
  board.width = size;
  board.height = size;
  context.putImageData(new ImageData(canvasPicture(cells), size, size), 0, 0);
}

function show(canvas) {
  size = canvas.size;
  cells = canvas.cells;
  nextAt = canvas.nextAt ? new Date(canvas.nextAt) : null;
  draw();
  tick();
}

// tick says when the next cell may be painted; a guest is asked to sign in.
function tick() {
  const status = $('status');
  if (!me) {
    status.replaceChildren(el('a', { href: '/', textContent: tr('Войдите') }), tr(', чтобы рисовать.'));
    return;
  }

  const left = nextAt ? Math.ceil((nextAt - Date.now()) / 1000) : 0;
  status.textContent = left > 0
    ? tr('Следующая клетка — через {0}.', `${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`)
    : tr('Можно закрасить клетку.');
  status.classList.toggle('canvas-ready', left <= 0);
}

function showPalette() {
  $('palette').replaceChildren(...canvasColors.map((hex, i) => {
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

function showZoom() {
  for (const z of zooms) board.classList.toggle(`canvas-zoom-${z}`, z === zooms[zoom]);
}

// tell says who painted a cell, or that nobody has.
async function tell({ x, y }) {
  const place = tr('Клетка {0}, {1}', x + 1, y + 1);
  try {
    const cell = await api.canvasCell(x, y);
    $('cell').replaceChildren(place, ': ',
      cell.user ? el('a', { href: profilePath(cell.user), textContent: fullName(cell.user) }) : tr('страница удалена'),
      ', ', formatPostDate(cell.paintedAt), '.');
  } catch (e) {
    if (e.status !== 404) throw e;
    $('cell').textContent = tr('{0}: её ещё никто не закрашивал.', place);
  }
}

// paint colors the cell at once and asks the server; a refusal puts the
// cell back.
async function paint({ x, y }) {
  const bytes = [...atob(cells)].map((c) => c.charCodeAt(0));
  const before = bytes[y * size + x];
  bytes[y * size + x] = color;
  const was = cells;
  cells = btoa(String.fromCharCode(...bytes));
  draw();

  try {
    const done = await api.paintCanvas(x, y, color);
    nextAt = new Date(done.nextAt);
    hideError($('error'));
  } catch (e) {
    if (bytes[y * size + x] !== before) {
      cells = was;
      draw();
    }
    showError($('error'), e);
  }
  tick();
}

function cellOf(event) {
  const box = board.getBoundingClientRect();

  return canvasCellAt(event.clientX - box.left, event.clientY - box.top, box.width, size);
}

board.addEventListener('click', (event) => {
  const cell = cellOf(event);
  if (looking || !me) {
    looking = false;
    $('look').classList.remove('active');
    tell(cell).catch((e) => showError($('error'), e));
    return;
  }
  if (nextAt && nextAt > Date.now()) {
    tell(cell).catch((e) => showError($('error'), e));
    return;
  }
  paint(cell);
});

// A mouse says who painted the cell under it; a finger asks by "Кто
// закрасил клетку?".
let hovered = '';
let hoverTimer = 0;
board.addEventListener('mousemove', (event) => {
  if (isPhone()) return;
  const cell = cellOf(event);
  const key = `${cell.x},${cell.y}`;
  if (key === hovered) return;
  hovered = key;
  clearTimeout(hoverTimer);
  hoverTimer = setTimeout(() => tell(cell).catch(() => {}), 300);
});

$('look').addEventListener('click', (event) => {
  event.preventDefault();
  looking = !looking;
  $('look').classList.toggle('active', looking);
  if (looking) $('cell').textContent = tr('Нажмите на клетку, чтобы узнать, кто её закрасил.');
});

$('zoom-in').addEventListener('click', (event) => {
  event.preventDefault();
  zoom = Math.min(zooms.length - 1, zoom + 1);
  showZoom();
});

$('zoom-out').addEventListener('click', (event) => {
  event.preventDefault();
  zoom = Math.max(0, zoom - 1);
  showZoom();
});

async function refresh() {
  if (document.hidden) return;
  try {
    const canvas = await api.canvas();
    // A cell painted meanwhile and not yet answered stays as drawn.
    show({ ...canvas, nextAt: nextAt && nextAt > Date.now() ? nextAt.toISOString() : canvas.nextAt });
  } catch {
    // The next refresh tries again.
  }
}

me = await currentUser().catch(() => null);
renderChrome(me);

try {
  show(await answered);
  showPalette();
  showZoom();
  $('board').hidden = false;
  setInterval(tick, 1000);
  setInterval(refresh, refreshEvery);
} catch (e) {
  showError($('error'), e);
}
