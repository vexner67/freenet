// openvk's "Граффити": a picture drawn with the mouse or a finger and
// attached to a post, a comment or a message as a photo. The strokes are
// kept, so the last one can be taken back ("Отменить") and the picture is
// drawn again at twice the size when it is saved.

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

// stroke is one line drawn without lifting the pen.
export function stroke(color, width, opacity, at) {
  return { color, width, opacity, points: [at] };
}

// drawStrokes draws strokes on a 2D context scaled by scale: through the
// middles of the points, so the line is smooth, and a dot for a click.
export function drawStrokes(ctx, strokes, scale = 1) {
  for (const s of strokes) {
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
  const ctx = canvas.getContext('2d');
  const strokes = [];
  let drawing = null;
  let color = GRAFFITI_COLORS[0];

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
    const outCtx = out.getContext('2d');
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

  const saveButton = button(tr('Прикрепить'), save);

  modal.box.classList.add('graffiti-dialog');
  modal.body.append(
    error,
    canvas,
    el('div', { className: 'graffiti-tools' },
      el('div', { className: 'graffiti-colors' }, ...swatches, picker),
      el('label', {}, tr('Толщина '), width),
      el('label', {}, tr('Интенсивность '), opacity),
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
