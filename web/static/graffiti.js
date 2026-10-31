// openvk's "Граффити": a picture drawn with the mouse or a finger and
// attached to a post, a comment or a message as a photo. The strokes are
// kept, so the last one can be taken back ("Отменить") and the picture is
// drawn again at twice the size when it is saved. "Заливка" pours a colour
// over the area under the pointer and is kept among the strokes.

import { el, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// The canvas as it is drawn on, and what is saved: twice as sharp.
export const GRAFFITI = { width: 560, height: 280, scale: 2 };
const BACKGROUND = '#fff';
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

// openGraffiti is the window to draw in; onDone gets the picture as a PNG
// file and may refuse it with an error, which the window shows.
export function openGraffiti(onDone) {
  const modal = openModal(tr('Граффити'));
  const error = el('div', { className: 'error', hidden: true });
  const canvas = el('canvas', { className: 'graffiti-canvas', width: GRAFFITI.width, height: GRAFFITI.height });
  // Fills read the picture back, and a software canvas does that fast.
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  const strokes = [];
  let drawing = null;
  let color = GRAFFITI_COLORS[0];
  let pouring = false;

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

  function redraw() {
    ctx.fillStyle = BACKGROUND;
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    drawStrokes(ctx, strokes);
  }

  function undo() {
    strokes.pop();
    redraw();
  }

  canvas.addEventListener('pointerdown', (event) => {
    if (event.button !== 0) return;

    if (pouring) {
      strokes.push(pour(color, Number(opacity.value) / 100, canvasPoint(event, canvas)));
      redraw();
      return;
    }

    drawing = stroke(color, Number(width.value), Number(opacity.value) / 100, canvasPoint(event, canvas));
    strokes.push(drawing);
    canvas.setPointerCapture(event.pointerId);
    redraw();
  });
  canvas.addEventListener('pointermove', (event) => {
    if (!drawing) return;

    drawing.points.push(canvasPoint(event, canvas));
    redraw();
  });
  const stop = () => { drawing = null; };
  canvas.addEventListener('pointerup', stop);
  canvas.addEventListener('pointercancel', stop);

  function onKey(event) {
    // Esc and a click beside the window close it without a word.
    if (!canvas.isConnected) {
      document.removeEventListener('keydown', onKey);
      return;
    }

    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z') {
      event.preventDefault();
      undo();
    }
  }
  document.addEventListener('keydown', onKey);
  const close = () => {
    document.removeEventListener('keydown', onKey);
    modal.close();
  };

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

  const fill = el('a', {
    className: 'graffiti-fill',
    textContent: tr('Заливка'),
    onclick: () => {
      pouring = !pouring;
      fill.classList.toggle('active', pouring);
      canvas.classList.toggle('pouring', pouring);
    },
  });

  const saveButton = button(tr('Прикрепить'), save);

  modal.box.classList.add('graffiti-dialog');
  modal.body.append(
    error,
    canvas,
    el('div', { className: 'graffiti-tools' },
      el('div', { className: 'graffiti-colors' }, ...swatches, picker),
      el('label', {}, tr('Толщина '), width),
      el('label', {}, tr('Интенсивность '), opacity),
      fill,
      el('a', { textContent: tr('Отменить'), title: 'Ctrl+Z', onclick: undo }),
      el('a', { textContent: tr('Очистить'), onclick: () => { strokes.length = 0; redraw(); } }),
    ),
  );
  modal.actions.append(saveButton, button(tr('Отмена'), close, { secondary: true }));

  pick(color);
  redraw();
}

// initGraffiti wires "Граффити" of a form: the drawing is
// attached as one more photo, and the window stays open when it is not.
export function initGraffiti({ link, attachments }) {
  link.addEventListener('click', () => openGraffiti((file) => attachments.add(file)));
}
