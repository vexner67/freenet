// openvk's "Фон страницы" of a person or a club: the two photos on the
// sides of the page, and the form that sets them ("Обзор" on each side of
// a scheme of the page, "Сохранить фон", "Удалить фон").

import { $, el, showError, hideError } from './layout.js';

const MAX_BYTES = 8 << 20;

// showBackdrop puts the photos behind the page, fading into white towards
// the middle. The images are set through the CSSOM, which the CSP allows,
// unlike style="".
export function showBackdrop(backdrop) {
  const { left, right } = backdrop ?? {};
  if (!left && !right) return;

  const node = el('div', { className: 'backdrop' }, el('div', { className: 'backdrop-dripper' }));
  node.style.backgroundImage = [left, right].map((photo) => (photo ? `url("${photo.large}")` : 'none')).join(', ');
  document.body.prepend(node);
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
        showError($('error'), 'Фотография больше 8 МБ.');
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
      showError($('error'), 'Не удалось сохранить фон: выберите хотя бы одно изображение.');
      return;
    }

    $('save').disabled = true;

    try {
      for (const side of sides) {
        backdrop = await upload(side, picked[side]);
        picked[side] = null;
      }
      showPreviews(backdrop);
      notice('Фон сохранён.');
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
      notice('Фон удалён.');
    } catch (e) {
      showError($('error'), e);
    } finally {
      $('remove').disabled = false;
    }
  });
}
