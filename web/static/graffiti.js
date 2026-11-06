// openvk's "Нарисовать граффити" (initGraffiti in Web/static/js/al_wall.js,
// Literally Canvas): a picture 640 by 480 drawn with the mouse or a finger,
// the tools in a column at its left and the colour, thickness and
// intensity under it, attached to a post, a comment or a message as a
// photo. The strokes are kept, so the last one can be taken back
// ("Отменить") and done again ("Повторить"), and the picture is drawn
// again at twice the size when it is saved. "Заливка" pours a colour over
// the area under the pointer. "Пиксели" is the site's own: the pen paints
// whole squares of a grid, as a picture of pixels.
//
// What is drawn is not drawn again: the finished strokes lie on a picture
// of their own, and a move of the pen draws that picture and the stroke it
// draws, once a frame — the pen keeps up however much is on the canvas.

import { el, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// The canvas as it is drawn on, and what is saved: twice as sharp.
export const GRAFFITI = { width: 640, height: 480, scale: 2 };
// PIXEL is the side of a square of "Пиксели", in pixels of the canvas.
export const PIXEL = 10;
const BACKGROUND = '#ffffff';
// The colours to pick from, as the palette of openvk's graffiti; any other
// comes from the colour picker.
export const GRAFFITI_COLORS = ['#000000', '#ffffff', '#7f7f7f', '#c3c3c3', '#880015', '#ed1c24', '#ff7f27', '#fff200', '#22b14c', '#00a2e8', '#3f48cc', '#a349a4'];
export const GRAFFITI_WIDTHS = { min: 1, max: 40, start: 6 };
// How far a colour may differ from the one poured over and still be poured
// over: the smoothed edges of lines are not left as a pale fringe.
const FILL_TOLERANCE = 96;

// stroke is one line drawn without lifting the pen.
export function stroke(color, width, opacity, at) {
  return { color, width, opacity, points: [at] };
}

// pixels is a stroke of "Пиксели": the squares of the grid it paints, each
// once, size squares wide.
export function pixels(color, size, opacity, at) {
  const s = { color, size, opacity, cells: [], seen: new Set() };
  addCells(s, at, at);
  return s;
}

// addCells paints the squares of s on the way from one point to the next,
// so a quick move leaves no gaps.
export function addCells(s, from, to) {
  const [x0, y0, x1, y1] = [from.x, from.y, to.x, to.y].map((v) => Math.floor(v / PIXEL));
  const steps = Math.max(Math.abs(x1 - x0), Math.abs(y1 - y0), 1);
  const reach = Math.floor((s.size - 1) / 2);

  for (let i = 0; i <= steps; i++) {
    const cx = Math.round(x0 + ((x1 - x0) * i) / steps) - reach;
    const cy = Math.round(y0 + ((y1 - y0) * i) / steps) - reach;
    for (let dx = 0; dx < s.size; dx++) {
      for (let dy = 0; dy < s.size; dy++) {
        const key = `${cx + dx},${cy + dy}`;
        if (s.seen.has(key)) continue;
        s.seen.add(key);
        s.cells.push([cx + dx, cy + dy]);
      }
    }
  }
}

// pour is a fill among the strokes: the colour poured at a point.
export function pour(color, opacity, at) {
  return { color, opacity, pour: at };
}

// floodFill lays the colour [r, g, b] with the given opacity over the area
// of RGBA pixels around x, y that is near the colour at that point, row by
// row, each pixel once.
export function floodFill(pixels, width, height, x, y, [r, g, b], opacity = 1) {
  const start = (y * width + x) * 4;
  const from = [pixels[start], pixels[start + 1], pixels[start + 2]];
  const seen = new Uint8Array(width * height);
  const near = (at) => !seen[at] && Math.abs(pixels[at * 4] - from[0]) + Math.abs(pixels[at * 4 + 1] - from[1]) + Math.abs(pixels[at * 4 + 2] - from[2]) <= FILL_TOLERANCE;
  const stack = [y * width + x];

  while (stack.length > 0) {
    const at = stack.pop();
    if (!near(at)) continue;

    const row = at - (at % width);
    let left = at;
    while (left > row && near(left - 1)) left--;
    let right = at;
    while (right < row + width - 1 && near(right + 1)) right++;

    for (let i = left; i <= right; i++) {
      seen[i] = 1;
      if (i >= width) stack.push(i - width);
      if (i < width * (height - 1)) stack.push(i + width);
    }
  }

  for (let i = 0; i < seen.length; i++) {
    if (!seen[i]) continue;

    pixels[i * 4] += (r - pixels[i * 4]) * opacity;
    pixels[i * 4 + 1] += (g - pixels[i * 4 + 1]) * opacity;
    pixels[i * 4 + 2] += (b - pixels[i * 4 + 2]) * opacity;
    pixels[i * 4 + 3] = 255;
  }
}

// pourOn does a fill on a 2D context scaled by scale.
function pourOn(ctx, s, scale) {
  const { width, height } = ctx.canvas;
  const image = ctx.getImageData(0, 0, width, height);
  const x = Math.min(width - 1, Math.floor(s.pour.x * scale));
  const y = Math.min(height - 1, Math.floor(s.pour.y * scale));
  const rgb = [1, 3, 5].map((i) => parseInt(s.color.slice(i, i + 2), 16));

  floodFill(image.data, width, height, x, y, rgb, s.opacity);
  ctx.putImageData(image, 0, 0);
}

// drawStrokes draws strokes on a 2D context scaled by scale: through the
// middles of the points, so the line is smooth, and a dot for a click.
export function drawStrokes(ctx, strokes, scale = 1) {
  for (const s of strokes) {
    if (s.pour) {
      pourOn(ctx, s, scale);
      continue;
    }

    if (s.cells) {
      ctx.save();
      ctx.globalAlpha = s.opacity;
      ctx.fillStyle = s.color;
      const side = PIXEL * scale;
      for (const [cx, cy] of s.cells) ctx.fillRect(cx * side, cy * side, side, side);
      ctx.restore();
      continue;
    }

    const [first, ...rest] = s.points;

    ctx.save();
    ctx.globalAlpha = s.opacity;
    ctx.strokeStyle = s.color;
    ctx.fillStyle = s.color;
    ctx.lineWidth = s.width * scale;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.beginPath();

    if (rest.length === 0) {
      ctx.arc(first.x * scale, first.y * scale, (s.width * scale) / 2, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
      continue;
    }

    ctx.moveTo(first.x * scale, first.y * scale);
    for (let i = 0; i < rest.length - 1; i++) {
      const mid = { x: (rest[i].x + rest[i + 1].x) / 2, y: (rest[i].y + rest[i + 1].y) / 2 };
      ctx.quadraticCurveTo(rest[i].x * scale, rest[i].y * scale, mid.x * scale, mid.y * scale);
    }
    const last = rest[rest.length - 1];
    ctx.lineTo(last.x * scale, last.y * scale);
    ctx.stroke();
    ctx.restore();
  }
}

// canvasPoint is where a pointer event is on the canvas in its own
// pixels, whatever size the page shows it at.
export function canvasPoint(event, canvas) {
  const box = canvas.getBoundingClientRect();

  return {
    x: ((event.clientX - box.left) / box.width) * canvas.width,
    y: ((event.clientY - box.top) / box.height) * canvas.height,
  };
}

// confirmExit is openvk's "Выход": whether to leave a drawing unsaved.
function confirmExit(onYes) {
  const modal = openModal(tr('Выход'));
  modal.body.append(el('p', { textContent: tr('Уверены, что хотите выйти?') }));
  modal.actions.append(
    button(tr('Нет'), modal.close, { secondary: true }),
    button(tr('Да'), () => {
      modal.close();
      onYes();
    }),
  );
}

// openGraffiti is the window to draw in; onDone gets the picture as a PNG
// file and may refuse it with an error, which the window shows.
export function openGraffiti(onDone) {
  const modal = openModal(tr('Нарисовать граффити'), { locked: true });
  const error = el('div', { className: 'error', hidden: true });
  const canvas = el('canvas', { className: 'graffiti-canvas', width: GRAFFITI.width, height: GRAFFITI.height });
  const ctx = canvas.getContext('2d');
  // The finished strokes; fills read it back, and a software canvas does
  // that fast.
  const base = el('canvas', { width: GRAFFITI.width, height: GRAFFITI.height });
  const baseCtx = base.getContext('2d', { willReadFrequently: true });
  const strokes = [];
  const undone = [];
  let drawing = null;
  let last = null;
  let frame = 0;
  let color = GRAFFITI_COLORS[0];
  let tool = 'pencil';
  let pixelMode = false;

  const picker = el('input', { type: 'color', value: color, title: tr('Другой цвет'), ariaLabel: tr('Цвет') });
  const width = el('input', { type: 'range', min: GRAFFITI_WIDTHS.min, max: GRAFFITI_WIDTHS.max, value: GRAFFITI_WIDTHS.start, ariaLabel: tr('Толщина') });
  const opacity = el('input', { type: 'range', min: 10, max: 100, value: 100, ariaLabel: tr('Интенсивность') });
  const swatches = GRAFFITI_COLORS.map((c) => {
    const swatch = el('a', { className: 'graffiti-swatch', title: c, onclick: () => pick(c) });
    // CSSOM, not a style attribute: the pages' CSP forbids those.
    swatch.style.backgroundColor = c;
    swatch.dataset.color = c;
    return swatch;
  });

  function pick(c) {
    color = c;
    picker.value = c;
    for (const s of swatches) s.classList.toggle('picked', s.dataset.color === c);
  }

  // show puts on the canvas the finished strokes, the one being drawn and,
  // in "Пиксели", the grid.
  function show() {
    frame = 0;
    ctx.drawImage(base, 0, 0);
    if (drawing) drawStrokes(ctx, [drawing]);
    if (!pixelMode) return;

    ctx.save();
    ctx.strokeStyle = 'rgba(0, 0, 0, 0.08)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (let x = PIXEL; x < canvas.width; x += PIXEL) {
      ctx.moveTo(x + 0.5, 0);
      ctx.lineTo(x + 0.5, canvas.height);
    }
    for (let y = PIXEL; y < canvas.height; y += PIXEL) {
      ctx.moveTo(0, y + 0.5);
      ctx.lineTo(canvas.width, y + 0.5);
    }
    ctx.stroke();
    ctx.restore();
  }

  const showSoon = () => {
    if (!frame) frame = requestAnimationFrame(show);
  };

  // rebuild draws the finished strokes anew: only when one is taken back.
  function rebuild() {
    baseCtx.fillStyle = BACKGROUND;
    baseCtx.fillRect(0, 0, base.width, base.height);
    drawStrokes(baseCtx, strokes);
    show();
  }

  function commit(s) {
    strokes.push(s);
    undone.length = 0;
    drawStrokes(baseCtx, [s]);
  }

  function undo() {
    if (strokes.length === 0) return;
    undone.push(strokes.pop());
    rebuild();
  }

  function redo() {
    if (undone.length === 0) return;
    const s = undone.pop();
    strokes.push(s);
    drawStrokes(baseCtx, [s]);
    show();
  }

  function clear() {
    if (strokes.length === 0) return;
    strokes.length = 0;
    undone.length = 0;
    rebuild();
  }

  const strength = () => Number(opacity.value) / 100;
  const penColor = () => (tool === 'eraser' ? BACKGROUND : color);

  canvas.addEventListener('pointerdown', (event) => {
    if (event.button !== 0) return;

    const at = canvasPoint(event, canvas);
    if (tool === 'fill') {
      commit(pour(color, strength(), at));
      show();
      return;
    }

    const alpha = tool === 'eraser' ? 1 : strength();
    drawing = pixelMode
      ? pixels(penColor(), Math.max(1, Math.round(Number(width.value) / PIXEL)), alpha, at)
      : stroke(penColor(), Number(width.value), alpha, at);
    last = at;
    canvas.setPointerCapture(event.pointerId);
    showSoon();
  });
  canvas.addEventListener('pointermove', (event) => {
    if (!drawing) return;

    const at = canvasPoint(event, canvas);
    if (drawing.cells) addCells(drawing, last, at);
    else drawing.points.push(at);
    last = at;
    showSoon();
  });
  const stop = () => {
    if (!drawing) return;
    commit(drawing);
    drawing = null;
    showSoon();
  };
  canvas.addEventListener('pointerup', stop);
  canvas.addEventListener('pointercancel', stop);

  function leave() {
    if (strokes.length === 0) {
      close();
      return;
    }
    confirmExit(close);
  }

  function onKey(event) {
    // A window over this one (the question of leaving) has its own keys.
    if (modal.box.parentElement !== document.body.lastElementChild) return;

    if (event.key === 'Escape') {
      leave();
      return;
    }
    if (!(event.ctrlKey || event.metaKey)) return;

    const key = event.key.toLowerCase();
    if (key === 'z' && !event.shiftKey) {
      event.preventDefault();
      undo();
    } else if (key === 'y' || (key === 'z' && event.shiftKey)) {
      event.preventDefault();
      redo();
    }
  }
  document.addEventListener('keydown', onKey);
  function close() {
    document.removeEventListener('keydown', onKey);
    if (frame) cancelAnimationFrame(frame);
    modal.close();
  }

  picker.addEventListener('input', () => pick(picker.value));

  async function save() {
    hideError(error);
    if (strokes.length === 0) {
      showError(error, tr('Нарисуйте что-нибудь.'));
      return;
    }

    saveButton.disabled = true;
    const out = el('canvas', { width: GRAFFITI.width * GRAFFITI.scale, height: GRAFFITI.height * GRAFFITI.scale });
    const outCtx = out.getContext('2d', { willReadFrequently: true });
    outCtx.fillStyle = BACKGROUND;
    outCtx.fillRect(0, 0, out.width, out.height);
    drawStrokes(outCtx, strokes, GRAFFITI.scale);

    try {
      const blob = await new Promise((resolve) => out.toBlob(resolve, 'image/png'));
      await onDone(new File([blob], 'graffiti.png', { type: 'image/png' }));
      close();
    } catch (e) {
      // The API's errors have a status; one of the page says itself.
      showError(error, e?.status ? e : e?.message ?? e);
      saveButton.disabled = false;
    }
  }

  // The tools, as the buttons of Literally Canvas: a square with its icon.
  const tools = {};
  const toolButton = (name, title, run) => el('a', { className: `graffiti-tool graffiti-tool-${name}`, title, onclick: run });
  const choose = (name) => {
    tool = name;
    for (const [n, b] of Object.entries(tools)) b.classList.toggle('selected', n === name);
    canvas.classList.toggle('pouring', name === 'fill');
  };
  tools.pencil = toolButton('pencil', tr('Карандаш'), () => choose('pencil'));
  tools.eraser = toolButton('eraser', tr('Ластик'), () => choose('eraser'));
  tools.fill = toolButton('fill', tr('Заливка'), () => choose('fill'));
  const pixelButton = toolButton('pixel', tr('Пиксели: рисовать квадратами сетки'), () => {
    pixelMode = !pixelMode;
    pixelButton.classList.toggle('selected', pixelMode);
    show();
  });

  const saveButton = button(tr('Сохранить'), save);

  modal.box.classList.add('graffiti-dialog');
  modal.body.append(
    error,
    el('div', { className: 'graffiti-draw' },
      el('div', { className: 'graffiti-picker' },
        el('div', { className: 'graffiti-tools' }, tools.pencil, tools.eraser, tools.fill, pixelButton),
        el('div', { className: 'graffiti-tools' },
          toolButton('undo', tr('Отменить') + ' (Ctrl+Z)', undo),
          toolButton('redo', tr('Повторить') + ' (Ctrl+Y)', redo),
          toolButton('clear', tr('Очистить'), clear))),
      el('div', { className: 'graffiti-main' },
        el('div', { className: 'graffiti-surface' }, canvas),
        el('div', { className: 'graffiti-options' },
          el('div', { className: 'graffiti-colors' }, ...swatches, picker),
          el('label', {}, tr('Толщина'), width),
          el('label', {}, tr('Интенсивность'), opacity)))),
  );
  modal.actions.append(saveButton, button(tr('Отмена'), leave, { secondary: true }));

  pick(color);
  choose('pencil');
  rebuild();
}

// initGraffiti wires "Граффити" of a form: the drawing is
// attached as one more photo, and the window stays open when it is not.
export function initGraffiti({ link, attachments }) {
  link.addEventListener('click', () => openGraffiti((file) => attachments.add(file)));
}
