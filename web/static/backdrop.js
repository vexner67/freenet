// openvk's "Фон страницы" of a person or a club: the two photos on the
// sides of the page, and the form that sets them ("Обзор" on each side of
// a scheme of the page, "Сохранить фон", "Удалить фон"); and, with Prime,
// a photo of a person's own behind the whole page.

import { $, el, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const MAX_BYTES = 8 << 20;

// showBackdrop puts the photos behind the page, fading into white towards
// the middle. The images are set through the CSSOM, which the CSP allows,
// unlike style="".
export function showBackdrop(backdrop) {
  const { left, right, cover } = backdrop ?? {};
  if (cover) {
    showCover(cover);
    return;
  }
  if (!left && !right) return;

  const node = el('div', { className: 'backdrop' }, el('div', { className: 'backdrop-dripper' }));
  node.style.backgroundImage = [left, right].map((photo) => (photo ? `url("${photo.large}")` : 'none')).join(', ');
  document.body.prepend(node);
}

// showCover puts a photo of the owner's, set with Prime, behind the whole
// page: it fills the screen without stretching and stays in place while
// the page scrolls, and the page lies on it in a white column, as on the
// sides. It is shown once loaded, so that a half-drawn picture does not
// creep in.
function showCover(photo) {
  const node = el('div', { className: 'backdrop backdrop-cover' }, el('div', { className: 'backdrop-dripper' }));
  const image = new Image();
  image.onload = () => {
    node.style.backgroundImage = `url("${photo.large}")`;
    document.body.prepend(node);
  };
  image.src = photo.large;
}

function showPreviews(backdrop) {
  for (const side of ['left', 'right']) {
    const photo = backdrop?.[side];
    $(side + '-preview').src = photo ? photo.small : '';
    $(side + '-preview').hidden = !photo;
  }
}

function notice(text) {
  $('notice').textContent = text;
  $('notice').hidden = false;
}

// initBackdropForm runs the form of the page (#form with #left-*, #right-*,
// #save, #remove, #notice, #error): upload(side, file) and remove() save
// and return the owner's new backdrop.
export function initBackdropForm(backdrop, { upload, remove }) {
  // picked are the files chosen for each side, not sent yet.
  const picked = { left: null, right: null };

  showPreviews(backdrop);

  for (const side of ['left', 'right']) {
    const file = $(side + '-file');

    $(side + '-browse').addEventListener('click', () => file.click());
    file.addEventListener('change', () => {
      const chosen = file.files[0];
      file.value = '';
      if (!chosen) return;

      if (chosen.size > MAX_BYTES) {
        showError($('error'), tr('Фотография больше 8 МБ.'));
        return;
      }

      picked[side] = chosen;
      // Shown at once from the file; the CSP allows data: images.
      const reader = new FileReader();
      reader.onload = () => {
        $(side + '-preview').src = reader.result;
        $(side + '-preview').hidden = false;
      };
      reader.readAsDataURL(chosen);
    });
  }

  $('form').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError($('error'));
    $('notice').hidden = true;

    const sides = ['left', 'right'].filter((side) => picked[side]);
    if (sides.length === 0) {
      showError($('error'), tr('Не удалось сохранить фон: выберите хотя бы одно изображение.'));
      return;
    }

    $('save').disabled = true;

    try {
      for (const side of sides) {
        backdrop = await upload(side, picked[side]);
        picked[side] = null;
      }
      showPreviews(backdrop);
      notice(tr('Фон сохранён.'));
    } catch (e) {
      showError($('error'), e);
    } finally {
      $('save').disabled = false;
    }
  });

  $('remove').addEventListener('click', async () => {
    hideError($('error'));
    $('notice').hidden = true;
    $('remove').disabled = true;

    try {
      backdrop = await remove();
      picked.left = picked.right = null;
      showPreviews(backdrop);
      notice(tr('Фон удалён.'));
    } catch (e) {
      showError($('error'), e);
    } finally {
      $('remove').disabled = false;
    }
  });
}

// initCoverForm runs "Фон из фотографии" (#cover-form): one photo behind
// the whole page, for Prime. prime tells whether the owner has it now;
// upload(file) and remove() save and return the owner's new backdrop.
export function initCoverForm(backdrop, prime, { upload, remove }) {
  let picked = null;
  const preview = $('cover-preview');
  const show = (b) => {
    preview.src = b?.cover ? b.cover.small : '';
    preview.hidden = !b?.cover;
  };

  show(backdrop);
  // Without Prime only taking the photo away is left.
  $('cover-prime').hidden = prime;
  $('cover-browse').disabled = $('cover-save').disabled = !prime;
  $('cover-remove').hidden = !backdrop?.cover;

  $('cover-browse').addEventListener('click', () => $('cover-file').click());
  $('cover-file').addEventListener('change', () => {
    const chosen = $('cover-file').files[0];
    $('cover-file').value = '';
    if (!chosen) return;

    if (chosen.size > MAX_BYTES) {
      showError($('error'), tr('Фотография больше 8 МБ.'));
      return;
    }

    picked = chosen;
    const reader = new FileReader();
    reader.onload = () => {
      preview.src = reader.result;
      preview.hidden = false;
    };
    reader.readAsDataURL(chosen);
  });

  const act = async (button, run, done) => {
    hideError($('error'));
    $('notice').hidden = true;
    button.disabled = true;

    try {
      backdrop = await run();
      picked = null;
      show(backdrop);
      $('cover-remove').hidden = !backdrop?.cover;
      notice(done);
    } catch (e) {
      showError($('error'), e);
    } finally {
      button.disabled = button === $('cover-save') && !prime;
    }
  };

  $('cover-form').addEventListener('submit', (event) => {
    event.preventDefault();
    if (!picked) {
      showError($('error'), tr('Выберите фотографию.'));
      return;
    }
    act($('cover-save'), () => upload(picked), tr('Фон сохранён.'));
  });
  $('cover-remove').addEventListener('click', () => act($('cover-remove'), remove, tr('Фон удалён.')));
}
