// The controls under the photo of a profile, for its owner, or of a
// community (club), for its admins: upload and remove call the API.

import { api } from './api.js';
import { $, avatarURL, showError, hideError } from './layout.js';

const MAX_BYTES = 8 << 20;

export function initPhoto(user, isOwner, {
  upload = api.uploadAvatar, remove = api.deleteAvatar, error = $('follow-error'), club = false,
} = {}) {
  const img = $('avatar');
  const status = $('avatar-status');

  function render(u) {
    img.src = avatarURL({ ...u, club }, 'large');
    $('avatar-delete').hidden = !u.avatar;
  }

  async function change(action, busyText) {
    hideError(error);
    status.textContent = busyText;
    status.hidden = false;

    try {
      render(await action());
    } catch (e) {
      showError(error, e);
    } finally {
      status.hidden = true;
    }
  }

  render(user);
  if (!isOwner) return;

  const file = $('avatar-file');

  $('avatar-actions').hidden = false;
  $('avatar-upload').addEventListener('click', () => file.click());
  $('avatar-delete').addEventListener('click', () => change(() => remove(), 'Удаление…'));

  file.addEventListener('change', () => {
    const picked = file.files[0];
    file.value = '';
    if (!picked) return;

    // Checked here too, so a huge file is not sent for nothing.
    if (picked.size > MAX_BYTES) {
      showError(error, 'Фотография больше 8 МБ.');
      return;
    }

    change(() => upload(picked), 'Загрузка…');
  });
}
