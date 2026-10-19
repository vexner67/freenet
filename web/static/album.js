// An album at /album/<id>, as openvk's: "Анна » Альбомы » Лето", how many
// photos with "Загрузить фотографию" and "Редактировать альбом" for whoever
// may change it, and the photos, oldest first, in a grey box; a cross on a
// photo deletes it for them, a click opens it over the page.

import { api, currentUser, started } from './api.js';
import { photosCount } from './labels.js';
import { $, adult, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { openViewer } from './viewer.js';
import { likeButton } from './posts.js';
import { openShare } from './share.js';
import { albumPath, albumsTitle, canChange, isKeptBySite, photoPath } from './albums.js';
import { tr } from './i18n.js';

const albumID = decodeURIComponent(location.pathname.slice('/album/'.length));

function renderHead(album, owner, editable) {
  document.title = tr('Альбом {0} | Freenet', album.name);
  $('title').replaceChildren(...albumsTitle(owner, album.name));
  $('count-link').href = albumPath(album);

  if (editable && !isKeptBySite(album)) {
    $('links').replaceChildren(
      ' | ', el('a', { href: albumPath(album) + '/upload', textContent: tr('Загрузить фотографию') }),
      ' | ', el('a', { href: albumPath(album) + '/edit', textContent: tr('Редактировать альбом') }),
    );
  }
}

// photoDetails is what the viewer shows under a photo of the album, as
// openvk's: its description, the heart, "Поделиться" and its comments.
function photoDetails(photo) {
  const comments = photo.comments > 0 ? tr('Комментарии ({0})', photo.comments) : tr('Комментировать');

  return el('div', {},
    photo.description ? el('p', { className: 'viewer-description', textContent: photo.description }) : null,
    el('div', { className: 'viewer-actions' },
      likeButton(photo, { canLike: Boolean(me), like: api.likePhoto, unlike: api.unlikePhoto, likers: api.photoLikers }),
      el('a', { href: photoPath(photo), textContent: comments }),
      me ? el('a', { textContent: tr('Поделиться'), onclick: () => openShare(photo, () => {}, { share: api.sharePhoto }) }) : null,
    ),
  );
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
    const modal = openModal(tr('Удалить фотографию'));
    modal.body.textContent = tr('Вы уверены, что хотите удалить эту фотографию из альбома?');
    modal.actions.append(
      button(tr('Да'), async () => {
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
      button(tr('Нет'), modal.close, { secondary: true }),
    );
  }

  function photoNode(photo) {
    const node = el('div', { className: 'album-photo' });
    const link = el('a', { href: photoPath(photo) },
      adult(el('img', { src: photo.small, alt: photo.description, loading: 'lazy' }), photo.nsfw));

    link.addEventListener('click', (event) => {
      event.preventDefault();
      openViewer(shown, shown.indexOf(photo), { details: photoDetails });
    });
    // Node.append writes false as text: only nodes go in.
    if (editable) node.append(el('a', { className: 'album-photo-delete', title: tr('Удалить фотографию'), textContent: '×', onclick: () => remove(photo, node) }));
    node.append(link);

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

// The page is asked for with who is signed in, in one round trip.
const wanted = started(api.album(albumID));
const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { album, owner, role } = await wanted;
  const editable = canChange(owner, role, me);

  renderHead(album, owner, editable);
  $('content').hidden = false;
  await initPhotos(album, editable);
} catch (e) {
  $('title').textContent = e.status === 404 ? tr('Альбом не найден') : tr('Ошибка');
  showError($('error'), e);
}
