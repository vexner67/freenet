// A photo of an album at /photo/<id>, as openvk's: "Анна » Альбомы » Лето
// » Фотография", the photo, its description, "Дата загрузки" and the
// heart, its comments, and on the right who uploaded it with
// "Редактировать" and "Удалить" for them, "Открыть оригинал" and
// "Поделиться".

import { api, currentUser } from './api.js';
import { formatPostDate, replyMention } from './labels.js';
import { $, adult, avatarURL, clubRef, el, fullName, pagePath, renderChrome, revealAdult, showError, hideError, autosize } from './layout.js';
import { button, openModal } from './modal.js';
import { albumPath, albumsTitle, canChange, isKeptBySite } from './albums.js';
import { commentNode, photoCommentHearts } from './comments.js';
import { likeButton } from './posts.js';
import { openShare } from './share.js';

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
  // A photo of an adult post is blurred until clicked, as the post is.
  adult($('image'), photo.nsfw).addEventListener('click', () => revealAdult($('image')));
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

// makeCover is "Обложка альбома": this photo shows its album instead of
// the first one.
async function makeCover(photo, album) {
  hideError($('error'));

  try {
    await api.setAlbumCover(album.id, photo.id);
    $('cover').replaceWith(el('span', { className: 'profile-link hint', textContent: 'Обложка альбома' }));
  } catch (e) {
    showError($('error'), e);
  }
}

// move puts the photo in another album of the same page: those people
// made, the first screen of them.
async function move(photo, album, owner) {
  const modal = openModal('Перенести в альбом');
  const albums = el('select', { ariaLabel: 'Альбом' });
  const error = el('div', { className: 'error', hidden: true });

  modal.body.append(error, el('div', { textContent: 'Альбом' }), albums);
  modal.actions.append(
    button('Перенести', async () => {
      if (!albums.value) return;

      try {
        await api.moveAlbumPhoto(photo.id, albums.value);
        location.reload();
      } catch (e) {
        showError(error, e);
      }
    }),
    button('Отмена', modal.close, { secondary: true }),
  );

  try {
    const page = await api.albums(owner.club ? { club: clubRef(owner), limit: 50 } : { nickname: owner.nickname, limit: 50 });
    const others = page.albums.filter((a) => a.id !== album.id && !isKeptBySite(a));

    albums.replaceChildren(...others.map((a) => el('option', { value: a.id, textContent: a.name })));
    if (others.length === 0) albums.replaceChildren(el('option', { value: '', textContent: 'Других альбомов нет' }));
  } catch (e) {
    showError(error, e);
  }
}

// initComments is openvk's "Комментарии" under a photo, oldest first, and
// the form for whoever signed in with a page; who uploaded the photo or
// runs its album deletes any of them.
function initComments(page) {
  const photo = page.photo;
  let cursor = '';
  let total = photo.comments;
  const moderates = Boolean(me) && (photo.mine || canChange(page.owner, page.role, me));

  function updateTitle() {
    $('comments-title').textContent = total > 0 ? `Комментарии (${total})` : 'Комментарии';
  }

  // The comment the form answers, null for none.
  let replyTo = null;

  // reply starts an answer as under a post: the name of the author first,
  // in place of the name of an earlier answer.
  function reply(comment) {
    const text = $('comment-text');
    const mention = replyTo ? replyMention(replyTo.author) : null;
    const rest = mention && text.value.startsWith(mention) ? text.value.slice(mention.length) : text.value;

    replyTo = comment;
    text.value = replyMention(comment.author) + rest;
    autosize(text);
    $('comment-reply-to').hidden = false;
    text.focus();
    text.setSelectionRange(text.value.length, text.value.length);
  }

  function cancelReply() {
    replyTo = null;
    $('comment-reply-to').hidden = true;
  }

  const node = (c) => commentNode({ ...c, photos: [] }, {
    hearts: photoCommentHearts,
    canLike: Boolean(me),
    onError: (e) => showError($('comments-error'), e),
    onReply: me?.profileCompleted && reply,
    whole: api.photoCommentText,
    onDelete: (c.mine || moderates) && (async (comment, shown) => {
      try {
        await api.deletePhotoComment(comment.id);
        shown.remove();
        total--;
        updateTitle();
      } catch (e) {
        showError($('comments-error'), e);
      }
    }),
  });

  async function loadMore() {
    try {
      const next = await api.photoComments(photo.id, cursor);
      $('comments').append(...next.comments.map(node));
      cursor = next.nextCursor;
      $('comments-more').hidden = !cursor;
    } catch (e) {
      showError($('comments-error'), e);
    }
  }

  $('comments-more').addEventListener('click', loadMore);
  if (me?.profileCompleted) {
    $('comment-form').hidden = false;
    $('comment-reply-cancel').addEventListener('click', cancelReply);
    $('comment-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      hideError($('comments-error'));

      try {
        const c = await api.commentPhoto(photo.id, $('comment-text').value, replyTo?.id);
        $('comments').append(node(c));
        $('comment-text').value = '';
        cancelReply();
        autosize($('comment-text'));
        total++;
        updateTitle();
      } catch (e) {
        showError($('comments-error'), e);
      }
    });
  }

  updateTitle();
  loadMore();
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const page = await api.albumPhoto(photoID);

  render(page);
  $('content').hidden = false;

  // As openvk: the heart for whoever signed in, "Поделиться" too.
  $('photo-like').replaceChildren(' | ', likeButton(page.photo, {
    canLike: Boolean(me), like: api.likePhoto, unlike: api.unlikePhoto, likers: api.photoLikers,
    onError: (e) => showError($('error'), e),
  }));
  if (me) {
    $('share').hidden = false;
    $('share').addEventListener('click', () => openShare(page.photo, () => {}, { share: api.sharePhoto }));
  }
  initComments(page);

  // Who uploaded it changes and deletes it, as on openvk.
  if (me && page.photo.mine) {
    $('edit').hidden = false;
    $('delete').hidden = false;
    $('edit').addEventListener('click', () => edit(page.photo));
    $('delete').addEventListener('click', () => remove(page.photo, page.album));
  }

  // Whoever may change the album chooses its cover and moves the photo.
  if (canChange(page.owner, page.role, me)) {
    $('cover').hidden = false;
    $('move').hidden = false;
    $('cover').addEventListener('click', () => makeCover(page.photo, page.album));
    $('move').addEventListener('click', () => move(page.photo, page.album, page.owner));
  }
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Фотография не найдена' : 'Ошибка';
  showError($('error'), e.status === 404 ? 'Фотография удалена или ещё не загружена.' : e);
}
