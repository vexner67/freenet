// openvk's "Редактировать альбом" at /album/<id>/edit: the tabs with
// "Добавить фотографии", the name and the description with "Сохранить",
// and "Вы также можете удалить альбом".

import { api, ApiError, currentUser } from './api.js';
import { $, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { albumPath, albumsPath, albumsTitle, canChange, isKeptBySite } from './albums.js';
import { tr } from './i18n.js';

const albumID = decodeURIComponent(location.pathname.slice('/album/'.length, -'/edit'.length));

function renderHead(album, owner) {
  document.title = tr('Редактировать альбом | Freenet');
  $('title').replaceChildren(...albumsTitle(owner, album.name));
}

// confirmDelete asks before the album goes with its photos; openvk deletes
// it at once.
function confirmDelete(album, owner) {
  const modal = openModal(tr('Удалить альбом'));
  modal.body.textContent = tr('Вы уверены, что хотите удалить альбом «{0}» со всеми фотографиями?', album.name);
  modal.actions.append(
    button(tr('Удалить'), async () => {
      try {
        await api.deleteAlbum(album.id);
        location.assign(albumsPath(owner));
      } catch (e) {
        modal.close();
        showError($('error'), e);
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  location.replace('/');
} else {
  try {
    const { album, owner, role } = await api.album(albumID);

    renderHead(album, owner);
    if (!canChange(owner, role, me)) throw new Error(tr('Недостаточно прав.'));
    if (isKeptBySite(album)) throw new Error(tr('Этот альбом сайт ведёт сам: его нельзя изменить.'));

    $('tab-edit').href = albumPath(album) + '/edit';
    $('tab-upload').href = albumPath(album) + '/upload';
    $('name').value = album.name;
    $('description').value = album.description;
    $('content').hidden = false;

    $('delete').addEventListener('click', () => confirmDelete(album, owner));
    $('form').addEventListener('submit', async (event) => {
      event.preventDefault();
      hideError($('error'));
      $('notice').hidden = true;
      $('save').disabled = true;

      try {
        const saved = await api.editAlbum(album.id, $('name').value, $('description').value);
        renderHead(saved.album, saved.owner);
        $('notice').textContent = tr('Изменения сохранены.');
        $('notice').hidden = false;
      } catch (e) {
        showError($('error'), e);
      } finally {
        $('save').disabled = false;
      }
    });
  } catch (e) {
    if (e.status === 404) $('title').textContent = tr('Альбом не найден');
    showError($('load-error'), e instanceof ApiError ? e : e.message);
  }
}
