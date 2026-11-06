// The window "Размер стикера" of the sticker studio: every sticker is
// shown in the same square, as openvk's 128 by 128 (LottieSticker in
// Web/static/js/messages/components/message.js), so before a picture
// becomes one its author places it in that square — whole at first,
// smaller with "Размер", moved by dragging. The window looks like
// openvk's "Загрузка новой фотографии" (avatar.js).

import { el } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// SIDE is the square the sticker is drawn in: twice what is shown, so it
// stays sharp on dense screens.
const SIDE = 256;

// fitSticker opens the window for file; it resolves with the square
// picture as PNG, or null when the author does not take it.
export function fitSticker(file) {
  return new Promise((resolve) => {
    const picture = new Image();
    const url = URL.createObjectURL(file);
    picture.onload = () => openFit(picture, (blob) => {
      URL.revokeObjectURL(url);
      resolve(blob);
    });
    picture.onerror = () => {
      URL.revokeObjectURL(url);
      resolve(null);
    };
    picture.src = url;
  });
}

function openFit(picture, done) {
  // Locked: the window ends only by its buttons, which answer for the picture.
  const modal = openModal(tr('Размер стикера'), { locked: true });
  const canvas = el('canvas', { className: 'sticker-fit-canvas', width: SIDE, height: SIDE });
  const size = el('input', { type: 'range', min: 20, max: 100, value: 100, ariaLabel: tr('Размер') });
  const ctx = canvas.getContext('2d');
  // The picture whole in the square, centred; the author shrinks and moves it.
  const whole = Math.min(SIDE / picture.naturalWidth, SIDE / picture.naturalHeight);
  let x = 0;
  let y = 0;
  let taken = false;

  const draw = () => {
    const scale = whole * Number(size.value) / 100;
    const w = picture.naturalWidth * scale;
    const h = picture.naturalHeight * scale;
    // The picture never leaves the square.
    x = Math.max(-(SIDE - w) / 2, Math.min((SIDE - w) / 2, x));
    y = Math.max(-(SIDE - h) / 2, Math.min((SIDE - h) / 2, y));
    ctx.clearRect(0, 0, SIDE, SIDE);
    ctx.drawImage(picture, (SIDE - w) / 2 + x, (SIDE - h) / 2 + y, w, h);
  };

  let from = null;
  canvas.addEventListener('pointerdown', (e) => {
    from = { x: e.clientX, y: e.clientY };
    canvas.setPointerCapture(e.pointerId);
  });
  canvas.addEventListener('pointermove', (e) => {
    if (!from) return;
    // The canvas may be shown smaller than it is drawn.
    const k = SIDE / canvas.getBoundingClientRect().width;
    x += (e.clientX - from.x) * k;
    y += (e.clientY - from.y) * k;
    from = { x: e.clientX, y: e.clientY };
    draw();
  });
  canvas.addEventListener('pointerup', () => { from = null; });
  size.addEventListener('input', draw);

  modal.box.classList.add('avatar-dialog', 'sticker-fit');
  modal.body.append(
    el('p', { textContent: tr('Все стикеры показываются в одном квадрате. Уменьшите изображение или сдвиньте его, если оно не помещается.') }),
    el('div', { className: 'sticker-fit-square' }, canvas),
    el('label', { className: 'sticker-fit-size' }, tr('Размер'), size),
  );

  const finish = (blob) => {
    if (taken) return;
    taken = true;
    modal.close();
    done(blob);
  };
  const take = button(tr('Загрузить'), () => {
    take.disabled = true;
    canvas.toBlob(finish, 'image/png');
  });
  modal.actions.append(button(tr('Отмена'), () => finish(null), { secondary: true }), take);
  draw();
}
