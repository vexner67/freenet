// openvk's "Фон страницы" of a club at /club/<ref>/backdrop, for its
// admins: "Обзор" on each side of a scheme of the page, "Сохранить фон"
// sends the sides picked, "Удалить фон" takes both away.

import { api, currentUser } from './api.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';

const MAX_BYTES = 8 << 20;

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/backdrop'.length));

// picked are the files chosen for each side, not sent yet.
const picked = { left: null, right: null };

function render(club) {
  const path = '/club/' + encodeURIComponent(clubRef(club));

  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ' » Фон страницы');
  $('tab-main').href = path + '/edit';
  $('tab-backdrop').href = path + '/backdrop';
  $('tab-members').href = path + '/members';

  for (const side of ['left', 'right']) {
    const photo = club.backdrop?.[side];
    $(side + '-preview').src = photo ? photo.small : '';
    $(side + '-preview').hidden = !photo;
  }
}

function notice(text) {
  $('notice').textContent = text;
  $('notice').hidden = false;
}

function initSide(side) {
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

async function save(club, event) {
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
      Object.assign(club, await api.setClubBackdrop(ref, side, picked[side]));
      picked[side] = null;
    }
    render(club);
    notice('Фон сохранён.');
  } catch (e) {
    showError($('error'), e);
  } finally {
    $('save').disabled = false;
  }
}

async function remove(club) {
  hideError($('error'));
  $('notice').hidden = true;
  $('remove').disabled = true;

  try {
    Object.assign(club, await api.deleteClubBackdrop(ref));
    picked.left = picked.right = null;
    render(club);
    notice('Фон удалён.');
  } catch (e) {
    showError($('error'), e);
  } finally {
    $('remove').disabled = false;
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  location.replace('/');
} else {
  try {
    const club = await api.club(ref);
    document.title = `${club.name} | Фон страницы | Freenet`;
    render(club);

    if (club.role !== 'admin' && club.role !== 'owner') {
      showError($('error'), 'Менять фон группы могут только её администраторы.');
    } else {
      initSide('left');
      initSide('right');
      $('form').addEventListener('submit', (event) => save(club, event));
      $('remove').addEventListener('click', () => remove(club));
      $('tabs').hidden = false;
      $('form').hidden = false;
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
    showError($('error'), e);
  }
}
