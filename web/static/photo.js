// The owner's controls under the profile photo.

import { api } from './api.js';
import { $, avatarURL, showError, hideError } from './layout.js';

const MAX_BYTES = 8 << 20;

export function initPhoto(user, isOwner) {
  const img = $('avatar');
  const error = $('follow-error');
  const status = $('avatar-status');

  function render(u) {
    img.src = avatarURL(u, 'large');
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
  $('avatar-delete').addEventListener('click', () => change(() => api.deleteAvatar(), 'Удаление…'));

  file.addEventListener('change', () => {
    const picked = file.files[0];
    file.value = '';
    if (!picked) return;

    // Checked here too, so a huge file is not sent for nothing.
    if (picked.size > MAX_BYTES) {
      showError(error, 'Фотография больше 8 МБ.');
      return;
    }

    change(() => api.uploadAvatar(picked), 'Загрузка…');
  });
}
