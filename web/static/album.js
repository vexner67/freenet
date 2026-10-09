// An album at /album/<id>, as openvk's: "Анна » Альбомы » Лето", how many
// photos with "Загрузить фотографию" and "Редактировать альбом" for whoever
// may change it, and the photos, oldest first, in a grey box; a cross on a
// photo deletes it for them, a click opens it over the page.

import { api, currentUser } from './api.js';
import { photosCount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { openViewer } from './viewer.js';
import { albumPath, albumsTitle, canChange, photoPath } from './albums.js';

const albumID = decodeURIComponent(location.pathname.slice('/album/'.length));

function renderHead(album, owner, editable) {
  document.title = `Альбом ${album.name} | Freenet`;
  $('title').replaceChildren(...albumsTitle(owner, album.name));
  $('count-link').href = albumPath(album);

  if (editable) {
    $('links').replaceChildren(
      ' | ', el('a', { href: albumPath(album) + '/upload', textContent: 'Загрузить фотографию' }),
      ' | ', el('a', { href: albumPath(album) + '/edit', textContent: 'Редактировать альбом' }),
    );
  }
}

function initPhotos(album, editable) {
  // shown are the photos on the page, in order, for the viewer.
  const shown = [];
  let total = album.photos;
  let cursor = '';

  function updateCount() {
    $('count').textContent = photosCount(total);
    $('photos').hidden = shown.length === 0;
    $('empty').hidden = shown.length > 0;
  }

  // remove asks as openvk's "Удалить фотографию" before the photo goes.
  function remove(photo, node) {
    const modal = openModal('Удалить фотографию');
    modal.body.textContent = 'Вы уверены, что хотите удалить эту фотографию из альбома?';
    modal.actions.append(
      button('Да', async () => {
        modal.close();
        hideError($('error'));

        try {
          await api.deleteAlbumPhoto(photo.id);
          shown.splice(shown.indexOf(photo), 1);
          node.remove();
          total--;
          updateCount();
        } catch (e) {
          showError($('error'), e);
        }
      }),
      button('Нет', modal.close, { secondary: true }),
    );
  }

  function photoNode(photo) {
    const node = el('div', { className: 'album-photo' });
    const link = el('a', { href: photoPath(photo) }, el('img', { src: photo.small, alt: photo.description, loading: 'lazy' }));

    link.addEventListener('click', (event) => {
      event.preventDefault();
      openViewer(shown, shown.indexOf(photo));
    });
    node.append(
      editable && el('a', { className: 'album-photo-delete', title: 'Удалить фотографию', textContent: '×', onclick: () => remove(photo, node) }),
      link,
    );

    return node;
  }

  async function loadMore() {
    hideError($('error'));

    try {
      const page = await api.albumPhotos(album.id, cursor);

      shown.push(...page.photos);
      $('photos').append(...page.photos.map(photoNode));
      total = page.total;
      cursor = page.nextCursor;
      $('more').hidden = !cursor;
      updateCount();
    } catch (e) {
      showError($('error'), e);
    }
  }

  $('more').addEventListener('click', loadMore);

  return loadMore();
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { album, owner, role } = await api.album(albumID);
  const editable = canChange(owner, role, me);

  renderHead(album, owner, editable);
  $('content').hidden = false;
  await initPhotos(album, editable);
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Альбом не найден' : 'Ошибка';
  showError($('error'), e);
}
