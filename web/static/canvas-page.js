// "Холст" (/canvas): one canvas of 100×100 cells for the whole site. Anyone
// signed in paints cells in one of 64 colors; anyone, a guest too, sees it
// and who painted a cell. Each cell comes as it is painted, over a socket of
// its own that needs no sign-in; while the socket is down the canvas comes
// anew every ten seconds. It works as the paint programs of the 2000s: a box
// of tools (each with its key), the colour in hand with the latest colours
// beside it, a cursor of the size of a cell. The brush paints while it is
// dragged, the hand moves the canvas, two fingers and the wheel zoom it. The
// strip over it counts what a person has painted, their combo and their rank;
// the count of cells and the rank are kept in this browser.

import { api, currentUser, started } from './api.js';
import { avatarURL, $, el, fullName, profilePath, renderChrome, showError, hideError, verifiedMark } from './layout.js';
import { canvasCellAt, canvasColors, canvasPalette, canvasPicture, formatPostDate, primeNameClass } from './labels.js';
import { helpButton, gameIcon } from './game.js';
import { popover } from './controls.js';
import { tr } from './i18n.js';

const answered = started(api.canvas());
const refreshEvery = 10_000;
const maxZoom = 40;
const buttonZoom = 1.6;
const wheelZoom = 0.002;
// How far a press moves before it is a drag and not a click.
const dragFrom = 6;
const hoverDelay = 120;
const reconnectAfter = 2_000;
// A cell painted this soon after another goes on the combo.
const comboGap = 1_500;
const undoSize = 200;
const pingGap = 120;
const liveWindow = 60_000;
const countKey = 'freenet.canvas.count';
// How long the brush rests after the server says too many cells.
const limitRest = 15_000;

let me = null;
let size = 100;
let cells = null;
let color = 3;
// What a press on the canvas does: brush, eraser, hand, pick or look.
let tool = 'brush';
const tools = ['brush', 'eraser', 'hand', 'pick', 'look'];
// The eraser paints the colour of a cell never painted.
const blank = 0;
// The colours used last, the latest first.
let recent = [];
const recentSize = 9;
// The cells painted and not answered yet: a refresh keeps them as drawn.
const unanswered = new Map();
// The socket is open; the cells that came while a refresh is on its way,
// which its older canvas must not undo.
let live = false;
let arrived = null;

const board = $('canvas');
const frame = $('frame');
const tip = $('tip');
const cursor = el('span', { className: 'canvas-cursor', hidden: true });
frame.append(cursor);
const context = board.getContext('2d');

// The ranks of a painter: how many cells make one.
const ranks = [
  [0, () => tr('Новичок')],
  [10, () => tr('Маляр')],
  [50, () => tr('Подмастерье')],
  [200, () => tr('Художник')],
  [500, () => tr('Мастер')],
  [1000, () => tr('Пикассо')],
  [5000, () => tr('Легенда холста')],
];

const rankAt = (count) => ranks.filter(([from]) => from <= count).length - 1;

function savedCount() {
  try {
    return Math.max(0, Number(localStorage.getItem(countKey)) || 0);
  } catch {
    return 0;
  }
}

let count = savedCount();
let combo = 0;
let lastPaint = 0;
let lastPing = 0;
// Cells others painted in the last minute, as times.
let others = [];
// What the undo button puts back.
const strokes = [];
// Until when the brush rests: the server refused a cell for the limit.
let restUntil = 0;

function hudCell(id, icon, name, tag = 'div') {
  const node = el(tag, { className: 'game-hud-cell', id: `hud-${id}`, type: tag === 'button' ? 'button' : undefined },
    el('span', { className: 'game-hud-name' }, gameIcon(icon), name),
    el('b', { className: 'game-hud-sum', id: `${id}-sum` }),
    el('span', { className: 'hint', id: `${id}-hint` }));

  return node;
}

function showRankHint() {
  const at = rankAt(count);
  const next = ranks[at + 1];

  $('rank-sum').textContent = ranks[at][1]();
  $('rank-hint').textContent = next ? tr('ещё {0} до «{1}»', next[0] - count, next[1]()) : tr('выше некуда');
}

function showRanks() {
  const pop = popover($('hud-rank'), tr('Звания'));
  if (!pop) return;

  pop.body.append(...ranks.map(([from, name]) => el('div', { className: count >= from ? 'canvas-rank got' : 'canvas-rank' },
    el('b', { textContent: name() }), el('span', { textContent: String(from) }))));
  pop.place();
}

function showHUD() {
  $('hud').replaceChildren(
    hudCell('count', '/static/icons/game/brush.svg', tr('Закрашено вами')),
    hudCell('combo', '/static/icons/game/combo.svg', tr('Комбо')),
    hudCell('rank', '/static/icons/game/crown.svg', tr('Звание'), 'button'),
    hudCell('live', '/static/icons/game/people.svg', tr('Рисуют сейчас')),
  );
  $('hud-rank').addEventListener('click', showRanks);
  $('count-sum').textContent = String(count);
  $('count-hint').textContent = tr('клеток');
  $('combo-sum').textContent = tr('×{0}', 0);
  $('combo-hint').textContent = tr('рисуйте без пауз');
  $('live-sum').textContent = '0';
  $('live-hint').textContent = tr('клеток за минуту');
  showRankHint();
}

function showLive() {
  const now = Date.now();

  others = others.filter((time) => now - time < liveWindow);
  $('live-sum').textContent = String(others.length);
}

// rewarded is what a painted cell gives: the count, the combo and, now and
// then, a rank.
function rewarded() {
  const now = Date.now();
  combo = now - lastPaint < comboGap ? combo + 1 : 1;
  lastPaint = now;
  count += 1;
  try {
    localStorage.setItem(countKey, String(count));
  } catch {
    // The count just stays for this page.
  }
  $('count-sum').textContent = String(count);
  $('combo-sum').textContent = tr('×{0}', combo);
  showRankHint();
}

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

// painted puts on the canvas a cell somebody has just painted; one pixel
// is drawn, as the whole picture would cost too much for each.
function painted({ x, y, color: value }) {
  if (!Number.isInteger(x) || !Number.isInteger(y) || !canvasColors[value]) return;

  cells = setCell(cells, x, y, value);
  context.fillStyle = canvasColors[value];
  context.fillRect(x, y, 1, 1);
  arrived?.push({ x, y, color: value });
  if (!unanswered.has(`${x},${y}`) && Date.now() - lastPaint > 400) othersPainted(x, y);
}

// othersPainted counts a cell of somebody else and rings it on the canvas,
// so that the canvas is seen alive.
function othersPainted(x, y) {
  const now = Date.now();

  others.push(now);
  showLive();
  if (now - lastPing < pingGap) return;

  lastPing = now;
  ring(x, y, 'canvas-ping');
}

function cellPixels() {
  return (frame.clientWidth * view.zoom) / size;
}

// ring is a mark over a cell that goes by itself.
function ring(x, y, className) {
  const side = cellPixels();
  const mark = el('span', { className });

  mark.style.left = `${view.x + x * side + side / 2}px`;
  mark.style.top = `${view.y + y * side + side / 2}px`;
  mark.addEventListener('animationend', () => mark.remove());
  frame.append(mark);
}

function showStatus() {
  $('status').replaceChildren(...(me
    ? [{ brush: tr('Рисуйте: нажмите и ведите по холсту.'), eraser: tr('Стирайте: нажмите и ведите по холсту.'), hand: tr('Двигайте холст, зажав его.'), pick: tr('Нажмите на клетку, чтобы взять её цвет.'), look: tr('Нажмите на клетку, чтобы узнать, кто её закрасил.') }[tool]]
    : [el('a', { href: '/', textContent: tr('Войдите') }), tr(', чтобы рисовать.')]));
}

function swatch(i) {
  const hex = canvasColors[i];
  const node = el('button', {
    type: 'button',
    className: i === color ? 'canvas-swatch active' : 'canvas-swatch',
    title: hex,
    onclick: () => {
      setColor(i);
      setTool('brush');
    },
  });
  node.style.backgroundColor = hex;

  return node;
}

function showPalette() {
  $('palette').replaceChildren(...canvasPalette().map(swatch));
  $('current').style.backgroundColor = canvasColors[color];
  cursor.style.backgroundColor = canvasColors[inHand()];
  // The places of the recent colours stay, filled or not.
  $('recent').replaceChildren(...Array.from({ length: recentSize }, (_, i) => (i < recent.length ? swatch(recent[i]) : el('span', { className: 'canvas-swatch empty' }))));
}

function setColor(i) {
  color = i;
  showPalette();
}

function remember(i) {
  if (recent[0] === i) return;

  recent = [i, ...recent.filter((c) => c !== i)].slice(0, recentSize);
  showPalette();
}

// inHand is the colour a press paints: the eraser's or the chosen one.
function inHand() {
  return tool === 'eraser' ? blank : color;
}

// painting says whether a press paints: the brush or the eraser.
function painting() {
  return (tool === 'brush' || tool === 'eraser') && me;
}

function setTool(next) {
  tool = next;
  cursor.style.backgroundColor = canvasColors[inHand()];
  for (const name of tools) $(`tool-${name}`).classList.toggle('active', name === tool);
  frame.dataset.tool = tool;
  cursor.hidden = true;
  hideTip();
  showStatus();
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
// cell back. It says whether the server took the cell.
async function paint({ x, y }, value = color, undoable = true) {
  const before = cellColor(x, y);
  if (before === value) return false;

  const key = `${x},${y}`;
  const mark = { x, y, value };

  unanswered.set(key, mark);
  cells = setCell(cells, x, y, value);
  context.fillStyle = canvasColors[value];
  context.fillRect(x, y, 1, 1);

  let taken = true;
  try {
    await api.paintCanvas(x, y, value);
    hideError($('error'));
    if (undoable) strokes.push({ x, y, before });
    if (strokes.length > undoSize) strokes.shift();
  } catch (e) {
    taken = false;
    if (e.status === 429) restUntil = Date.now() + limitRest;
    if (unanswered.get(key) === mark) {
      cells = setCell(cells, x, y, before);
      draw();
    }
    showError($('error'), e);
  }
  if (unanswered.get(key) === mark) unanswered.delete(key);

  return taken;
}

function cellColor(x, y) {
  return atob(cells).charCodeAt(y * size + x);
}

// brush paints a cell at once, and the server is asked after the cell is
// on the canvas; the rewards come when it takes the cell, so a cell the
// limit refused counts for nothing. While the limit lasts the brush rests.
function brush(cell) {
  const value = inHand();
  if (cellColor(cell.x, cell.y) === value || Date.now() < restUntil) return;

  if (tool === 'brush') remember(value);
  ring(cell.x, cell.y, 'canvas-pop');
  paint(cell, value).then((taken) => { if (taken) rewarded(); }).catch((e) => showError($('error'), e));
}

// stroke paints the cells between the last one and this one: a fast hand
// would leave gaps.
function stroke(cell) {
  const from = press.cell ?? press.first;
  const steps = Math.max(Math.abs(cell.x - from.x), Math.abs(cell.y - from.y), 1);

  for (let i = press.cell ? 1 : 0; i <= steps; i++) {
    brush({ x: Math.round(from.x + ((cell.x - from.x) * i) / steps), y: Math.round(from.y + ((cell.y - from.y) * i) / steps) });
  }
  press.cell = cell;
}

function undo() {
  const last = strokes.pop();
  if (!last) return;

  paint(last, last.before, false).catch((e) => showError($('error'), e));
}

function cellOf(event) {
  const box = board.getBoundingClientRect();

  return canvasCellAt(event.clientX - box.left, event.clientY - box.top, box.width, size);
}

function act(event) {
  const cell = cellOf(event);

  if (tool === 'pick') {
    setColor(cellColor(cell.x, cell.y));
    setTool('brush');
    return;
  }
  if (tool === 'look' || !me) {
    tell(cell, { x: event.clientX, y: event.clientY }, true).catch((e) => showError($('error'), e));
    return;
  }
  if (painting()) brush(cell);
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
    ? { x: event.clientX, y: event.clientY, moved: false, pinch: null, first: cellOf(event) }
    : press && { ...press, moved: true, pinched: true, pinch: pointers.size === 2 ? pinch() : null };
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
  if (press.moved && painting() && !press.pinched) {
    stroke(cellOf(event));
    return;
  }
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
  const cell = cellOf(event);

  if (painting()) {
    const side = cellPixels();

    cursor.hidden = false;
    cursor.style.width = `${side}px`;
    cursor.style.height = `${side}px`;
    cursor.style.transform = `translate(${view.x + cell.x * side}px, ${view.y + cell.y * side}px)`;
    return;
  }
  if (tool === 'pick' || tip.classList.contains('pinned')) return;

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
  cursor.hidden = true;
  if (!tip.classList.contains('pinned')) hideTip();
});

// A pinned line goes with a press anywhere else.
document.addEventListener('pointerdown', (event) => {
  if (tip.classList.contains('pinned') && !tip.contains(event.target)) hideTip();
});

for (const name of tools) $(`tool-${name}`).addEventListener('click', () => setTool(name));
$('zoom-in').addEventListener('click', () => zoomByButton(buttonZoom));
$('zoom-out').addEventListener('click', () => zoomByButton(1 / buttonZoom));
$('undo').addEventListener('click', undo);

function fit() {
  view.zoom = 1;
  place();
}

$('zoom-fit').addEventListener('click', fit);

// The keys of the tools, as in the paint programs.
const keys = {
  b: () => setTool('brush'), e: () => setTool('eraser'), h: () => setTool('hand'), i: () => setTool('pick'), w: () => setTool('look'),
  '+': () => zoomByButton(buttonZoom), '=': () => zoomByButton(buttonZoom), '-': () => zoomByButton(1 / buttonZoom), 0: fit, z: undo,
};

document.addEventListener('keydown', (event) => {
  if (event.target.closest?.('input, textarea, select') || event.altKey || event.metaKey) return;

  const run = keys[event.key.toLowerCase()];
  if (run && !(event.ctrlKey && event.key.toLowerCase() !== 'z')) {
    event.preventDefault();
    run();
  }
});

addEventListener('resize', place);

async function refresh() {
  if (document.hidden && !live) return;

  arrived = [];
  try {
    const canvas = await api.canvas();
    const late = arrived;

    arrived = null;
    show(canvas);
    late.forEach(painted);
  } catch {
    // The next refresh tries again.
  } finally {
    arrived = null;
  }
}

// connect holds the socket of the cells; each new one reads the canvas, as
// the cells painted while it was down never came.
function connect() {
  const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
  const socket = new WebSocket(`${scheme}://${location.host}/api/v1/canvas/live`);

  socket.addEventListener('open', () => {
    live = true;
    refresh();
  });
  socket.addEventListener('message', (message) => painted(JSON.parse(message.data)));
  socket.addEventListener('close', () => {
    live = false;
    setTimeout(connect, reconnectAfter);
  });
}

me = await currentUser().catch(() => null);
renderChrome(me);

try {
  show(await answered);
  showHUD();
  showPalette();
  setTool(me ? 'brush' : 'look');
  $('canvas-help').replaceWith(helpButton(tr('Один холст на весь сайт: закрашивайте клетки сколько хотите. Вместе можно нарисовать что угодно — договаривайтесь в сообщениях.')));
  $('board').hidden = false;
  place();
  connect();
  setInterval(() => {
    if (!live) refresh();
  }, refreshEvery);
  setInterval(showLive, refreshEvery);
} catch (e) {
  showError($('error'), e);
}
