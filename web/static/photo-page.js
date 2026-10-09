// A photo of an album at /photo/<id>, as openvk's: "Анна » Альбомы » Лето
// » Фотография", the photo, its description and "Дата загрузки", and on
// the right who uploaded it with "Редактировать" and "Удалить" for them
// and "Открыть оригинал".

import { api, currentUser } from './api.js';
import { formatPostDate } from './labels.js';
import { $, avatarURL, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { albumPath, albumsTitle } from './albums.js';

const photoID = decodeURIComponent(location.pathname.slice('/photo/'.length));

function showDescription(text) {
  $('description').textContent = text;
  $('description').hidden = !text;
  $('image').alt = text;
}

function render({ photo, author, album, owner }) {
  document.title = 'Фотография | Freenet';
  $('title').replaceChildren(...albumsTitle(owner, el('a', { href: albumPath(album), textContent: album.name }), 'Фотография'));

  $('image').src = photo.large;
  showDescription(photo.description);
  $('uploaded-at').textContent = formatPostDate(photo.createdAt);

  $('author').href = pagePath(author);
  $('author-avatar').src = avatarURL(author, 'small');
  $('author-name').textContent = fullName(author);
  $('original').href = photo.large;
}

// edit is openvk's "Изменить фотографию": the description in a box.
function edit(photo) {
  const modal = openModal('Изменить фотографию');
  const text = el('textarea', { maxLength: 255, rows: 4, value: $('description').textContent });

  modal.body.append(el('div', { textContent: 'Описание' }), text);
  modal.actions.append(
    button('Сохранить', async () => {
      hideError($('error'));

      try {
        const saved = await api.describeAlbumPhoto(photo.id, text.value);
        showDescription(saved.photo.description);
        modal.close();
      } catch (e) {
        modal.close();
        showError($('error'), e);
      }
    }),
    button('Отмена', modal.close, { secondary: true }),
  );
  text.focus();
}

function remove(photo, album) {
  const modal = openModal('Удалить фотографию');
  modal.body.textContent = 'Вы уверены, что хотите удалить эту фотографию из альбома?';
  modal.actions.append(
    button('Да', async () => {
      try {
        await api.deleteAlbumPhoto(photo.id);
        location.assign(albumPath(album));
      } catch (e) {
        modal.close();
        showError($('error'), e);
      }
    }),
    button('Нет', modal.close, { secondary: true }),
  );
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const page = await api.albumPhoto(photoID);

  render(page);
  $('content').hidden = false;

  // Who uploaded it changes and deletes it, as on openvk.
  if (me && page.photo.mine) {
    $('edit').hidden = false;
    $('delete').hidden = false;
    $('edit').addEventListener('click', () => edit(page.photo));
    $('delete').addEventListener('click', () => remove(page.photo, page.album));
  }
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Фотография не найдена' : 'Ошибка';
  showError($('error'), e.status === 404 ? 'Фотография удалена или ещё не загружена.' : e);
}
