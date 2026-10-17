// openvk's "Загрузка новой фотографии": pick a picture, choose the square
// the page shows, turn the picture if it lies on its side and, as openvk
// offers, post it on the wall. The chosen square is cut here and sent as
// the photo, as openvk does; the server keeps the middle square of what it
// gets, which is all of it.
//
// The picture and the square are drawn on one canvas, so nothing needs
// inline styles (the pages' CSP forbids them).

import { button, openModal } from './modal.js';
import { el, showError } from './layout.js';
import { cropSquare, fitInside, movedSquare, resizedSquare } from './crop.js';

const MAX_BYTES = 8 << 20;
// The picture is worked on at most this big: enough for the 400×400
// avatar, and a phone's 50 megapixels would be slow to turn.
const WORK_SIDE = 1600;
// What is sent: twice the avatar, so it stays sharp after scaling.
const UPLOAD_SIDE = 800;
const VIEW = { width: 440, height: 300 };
// The smallest square, in pixels of the picture worked on: the server's
// minimum, which a smaller square would be scaled up to anyway.
const MIN_SIDE = 50;
// The handle at the bottom right corner that resizes the square.
const HANDLE = 10;

// openAvatarDialog asks for the photo and calls upload(blob) with the
// chosen square; it closes once upload resolves and shows its error
// otherwise. Unlike openvk, no post tells of the new photo.
export function openAvatarDialog({ club = false, chat = false, upload }) {
  const modal = openModal('Загрузка новой фотографии');
  modal.box.classList.add('avatar-dialog');

  const file = el('input', { type: 'file', accept: 'image/jpeg,image/png,image/gif,image/webp', hidden: true });
  const error = el('p', { className: 'error', hidden: true });

  modal.body.append(
    el('p', { textContent: intro(club, chat) }),
    el('p', { textContent: 'Вы можете загрузить изображение в формате JPG, GIF, PNG или WebP.' }),
    el('p', { className: 'center' }, button('Обзор', () => file.click()), file),
    el('p', { className: 'hint', textContent: 'Если возникают проблемы с загрузкой, попробуйте выбрать фотографию меньшего размера.' }),
    error,
  );
  modal.actions.append(button('Отмена', modal.close, { secondary: true }));

  file.addEventListener('change', async () => {
    const picked = file.files[0];
    file.value = '';
    if (!picked) return;

    if (picked.size > MAX_BYTES) {
      showError(error, 'Фотография больше 8 МБ.');
      return;
    }

    try {
      showCropper(modal, await loadImage(picked), { club, chat, upload });
    } catch {
      showError(error, 'Не удалось открыть изображение.');
    }
  });
}

// loadImage reads the file as a data: URL, which the CSP allows for
// images; the browser turns a photo upright by its EXIF orientation.
function loadImage(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error);
    reader.onload = () => {
      const img = new Image();
      img.onload = () => resolve(img);
      img.onerror = reject;
      img.src = reader.result;
    };
    reader.readAsDataURL(blob);
  });
}

// upright draws img turned by quarter turns clockwise, at most WORK_SIDE.
function upright(img, quarters) {
  const turned = quarters % 2 === 1;
  const w = turned ? img.naturalHeight : img.naturalWidth;
  const h = turned ? img.naturalWidth : img.naturalHeight;
  const size = fitInside(w, h, WORK_SIDE, WORK_SIDE);

  const canvas = el('canvas', { width: size.width, height: size.height });
  const ctx = canvas.getContext('2d');
  ctx.translate(size.width / 2, size.height / 2);
  ctx.rotate((quarters * Math.PI) / 2);
  const drawW = turned ? size.height : size.width;
  const drawH = turned ? size.width : size.height;
  ctx.drawImage(img, -drawW / 2, -drawH / 2, drawW, drawH);

  return canvas;
}

function intro(club, chat) {
  if (chat) return 'Фотография поможет участникам быстрее найти беседу в списке.';

  return club
    ? 'Хорошее фото сделает Ваше сообщество более узнаваемым.'
    : 'Друзьям будет проще узнать Вас, если вы загрузите свою настоящую фотографию.';
}

function shownWhere(club, chat) {
  if (chat) return 'Выбранная область будет показываться в беседе и в списке диалогов.';

  return club
    ? 'Выбранная область будет показываться на странице сообщества.'
    : 'Выбранная область будет показываться на вашей странице.';
}

function showCropper(modal, img, { club, chat, upload }) {
  let quarters = 0;
  let picture = upright(img, quarters);
  let square = cropSquare(picture.width, picture.height);

  const view = el('canvas', { className: 'avatar-crop' });
  const error = el('p', { className: 'error', hidden: true });
  let scale = 1;

  function layout() {
    const size = fitInside(picture.width, picture.height, VIEW.width, VIEW.height, { grow: true });
    view.width = size.width;
    view.height = size.height;
    scale = size.width / picture.width;
    draw();
  }

  function draw() {
    const ctx = view.getContext('2d');
    // Whole pixels: the dark parts would show seams between them otherwise.
    const [x, y, side] = [square.x, square.y, square.side].map((v) => Math.round(v * scale));

    ctx.drawImage(picture, 0, 0, view.width, view.height);
    // Darken all but the square, as openvk's cropper does.
    ctx.fillStyle = 'rgba(0, 0, 0, 0.5)';
    ctx.fillRect(0, 0, view.width, y);
    ctx.fillRect(0, y + side, view.width, view.height - y - side);
    ctx.fillRect(0, y, x, side);
    ctx.fillRect(x + side, y, view.width - x - side, side);
    ctx.strokeStyle = '#fff';
    ctx.strokeRect(x + 0.5, y + 0.5, side - 1, side - 1);
    ctx.fillStyle = '#fff';
    ctx.fillRect(x + side - HANDLE, y + side - HANDLE, HANDLE, HANDLE);
  }

  function turn(by) {
    quarters = (quarters + by + 4) % 4;
    picture = upright(img, quarters);
    square = cropSquare(picture.width, picture.height);
    layout();
  }

  // Dragging inside the square moves it, the corner handle resizes it.
  let drag = null;
  // shown is how many pixels of the canvas a pixel of the page is: a
  // phone shows the canvas smaller than it is drawn.
  const shown = () => view.width / (view.clientWidth || view.width);
  const pictureAt = (event) => ({ x: (event.offsetX * shown()) / scale, y: (event.offsetY * shown()) / scale });
  view.addEventListener('pointerdown', (event) => {
    const at = pictureAt(event);
    const handle = (HANDLE * shown()) / scale;
    const inside = at.x >= square.x && at.x <= square.x + square.side && at.y >= square.y && at.y <= square.y + square.side;
    if (!inside) return;

    const resizing = at.x >= square.x + square.side - handle && at.y >= square.y + square.side - handle;
    drag = { from: at, start: square, resizing };
    view.setPointerCapture(event.pointerId);
  });
  view.addEventListener('pointermove', (event) => {
    if (!drag) return;

    const at = pictureAt(event);
    const dx = at.x - drag.from.x;
    const dy = at.y - drag.from.y;
    const bounds = { width: picture.width, height: picture.height };
    square = drag.resizing
      ? resizedSquare(drag.start, Math.max(dx, dy), bounds, MIN_SIDE)
      : movedSquare(drag.start, dx, dy, bounds);
    draw();
  });
  view.addEventListener('pointerup', () => { drag = null; });
  view.addEventListener('pointercancel', () => { drag = null; });

  async function send(sendButton) {
    const side = Math.min(Math.max(Math.round(square.side), MIN_SIDE), UPLOAD_SIDE);
    const out = el('canvas', { width: side, height: side });
    const ctx = out.getContext('2d');
    // JPEG has no transparency: white, as the server shows it.
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, side, side);
    ctx.drawImage(picture, square.x, square.y, square.side, square.side, 0, 0, side, side);

    sendButton.disabled = true;
    try {
      const blob = await new Promise((resolve) => out.toBlob(resolve, 'image/jpeg', 0.92));
      await upload(blob);
      modal.close();
    } catch (e) {
      showError(error, e);
      sendButton.disabled = false;
    }
  }

  modal.body.replaceChildren(
    el('p', { textContent: shownWhere(club, chat) }),
    el('p', { textContent: 'Если изображение ориентировано неправильно, фотографию можно повернуть.' }),
    view,
    el('div', { className: 'avatar-turn' },
      button('Повернуть влево', () => turn(-1), { secondary: true }),
      button('Повернуть вправо', () => turn(1), { secondary: true })),
    error,
  );

  const sendButton = button('Загрузить', () => send(sendButton));
  modal.actions.replaceChildren(button('Отмена', modal.close, { secondary: true }), sendButton);

  layout();
}
