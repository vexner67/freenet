// The controls on the photo of a profile, for its owner, or of a
// community (club), for its admins, shown as openvk shows them when the
// photo is pointed at: upload (through openvk's dialog, avatar.js) along
// its bottom and remove in its corner call the API.

import { api } from './api.js';
import { openAvatarDialog } from './avatar.js';
import { $, avatarURL, showError, hideError } from './layout.js';

export function initPhoto(user, isOwner, {
  upload = api.uploadAvatar, remove = api.deleteAvatar, error = $('follow-error'), club = false,
} = {}) {
  const img = $('avatar');
  const status = $('avatar-status');

  function render(u) {
    img.src = avatarURL({ ...u, club }, 'large');
    $('avatar-delete').hidden = !u.avatar;
    $('avatar-upload').textContent = u.avatar ? 'Загрузить новую фотографию' : 'Поставить изображение';
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

  $('avatar-actions').hidden = false;
  $('avatar-upload').addEventListener('click', () => openAvatarDialog({
    club,
    // The dialog shows what went wrong itself and stays open.
    upload: async (blob) => {
      hideError(error);
      render(await upload(blob));
    },
  }));
  $('avatar-delete').addEventListener('click', () => change(() => remove(), 'Удаление…'));
}
