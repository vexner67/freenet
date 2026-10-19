// openvk's "Добавить фотографии" at /album/<id>/upload: "Загрузить
// фотографии" picks up to 10 files, each uploads at once and shows with
// "Описание:" and "Удалить"; "Завершить загрузку" puts them in the album
// with their descriptions and opens it.

import { api, ApiError, currentUser } from './api.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { albumPath, albumsTitle, canChange, isKeptBySite } from './albums.js';
import { tr } from './i18n.js';

const MAX_PHOTOS = 10;
const MAX_BYTES = 8 << 20;

const albumID = decodeURIComponent(location.pathname.slice('/album/'.length, -'/upload'.length));

function renderHead(album, owner) {
  document.title = tr('Загрузить фотографию | Freenet');
  $('title').replaceChildren(...albumsTitle(owner, el('a', { href: albumPath(album), textContent: album.name }), tr('Загрузить фотографию')));
  $('tab-edit').href = albumPath(album) + '/edit';
  $('tab-upload').href = albumPath(album) + '/upload';
}

function initUpload(album) {
  // Entries keep the order the photos were picked in; an entry has its
  // photo once the upload is done. Uploads go one by one: the album
  // orders photos by when media got them, and so keeps the picked order.
  let entries = [];
  let queue = Promise.resolve();

  const ready = () => entries.filter((e) => e.photo);

  function render() {
    $('uploaded').replaceChildren(...entries.map((e) => e.node));
    $('finish').hidden = ready().length === 0;
  }

  function entryNode(entry) {
    const image = el('div', { className: 'uploaded-image' }, el('div', { className: 'attachment-loading', textContent: tr('Загрузка…') }));

    entry.description = el('textarea', { maxLength: 255, rows: 4 });
    entry.image = image;
    entry.node = el('div', { className: 'uploaded-photo' },
      el('label', { className: 'uploaded-description' }, el('span', { textContent: tr('Описание:') }), entry.description),
      image,
    );
  }

  async function upload(entry, file) {
    try {
      entry.photo = await api.uploadPhoto(file);
      entry.image.replaceChildren(
        el('img', { src: entry.photo.small, alt: '' }),
        el('a', {
          className: 'profile-link',
          textContent: tr('Удалить'),
          onclick: () => {
            entries = entries.filter((e) => e !== entry);
            render();
          },
        }),
      );
    } catch (e) {
      entries = entries.filter((x) => x !== entry);
      showError($('error'), e);
    }
    render();
  }

  // The first pick hides the box with the limits and keeps the button
  // above the photos, as openvk does.
  function pick() {
    hideError($('error'));

    const files = [...$('file').files];
    $('file').value = '';

    if (entries.length + files.length > MAX_PHOTOS) {
      showError($('error'), tr('Вы можете загружать до 10 фотографий за один раз.'));
      return;
    }

    if (files.some((f) => f.size > MAX_BYTES)) {
      showError($('error'), tr('Фотография больше 8 МБ.'));
      return;
    }

    if (files.length > 0 && !$('intro').hidden) {
      $('intro').hidden = true;
      $('again').append($('pick'));
    }

    for (const file of files) {
      const entry = { photo: null };
      entryNode(entry);
      entries.push(entry);
      queue = queue.then(() => upload(entry, file));
    }

    render();
  }

  async function finish() {
    hideError($('error'));

    if (entries.some((e) => !e.photo)) {
      showError($('error'), tr('Дождитесь, пока загрузятся фотографии.'));
      return;
    }

    $('finish').disabled = true;

    try {
      await api.addAlbumPhotos(album.id, ready().map((e) => ({ id: e.photo.id, description: e.description.value })));
      location.assign(albumPath(album));
    } catch (e) {
      showError($('error'), e);
      $('finish').disabled = false;
    }
  }

  $('pick').addEventListener('click', () => $('file').click());
  $('file').addEventListener('change', pick);
  $('finish').addEventListener('click', finish);
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

    $('content').hidden = false;
    initUpload(album);
  } catch (e) {
    if (e.status === 404) $('title').textContent = tr('Альбом не найден');
    showError($('load-error'), e instanceof ApiError ? e : e.message);
  }
}
