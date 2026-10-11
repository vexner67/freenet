// A note at /note/<id>, as openvk's: "Анна » Заметки » Название", the
// note whole with "Удалить" and "Редактировать" for its author, and the
// comments under it, oldest first, with the form for whoever signed in
// with a page. The author of the note deletes any comment.

import { api, currentUser } from './api.js';
import { $, renderChrome, showError, hideError, autosize } from './layout.js';
import { commentNode } from './comments.js';
import { confirmDelete, noteNode, notesPath, notesTitle } from './notes.js';

const noteID = decodeURIComponent(location.pathname.slice('/note/'.length));

function initComments(note, me) {
  let cursor = '';
  let total = note.comments;

  function updateTitle() {
    $('comments-title').textContent = total > 0 ? `Комментарии (${total})` : 'Комментарии';
  }

  const node = (c) => commentNode(c, {
    withLike: false,
    whole: api.noteCommentText,
    onDelete: (c.mine || note.mine) && (async (comment, shown) => {
      hideError($('comments-error'));

      try {
        await api.deleteNoteComment(comment.id);
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
      const next = await api.noteComments(note.id, cursor);
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
    $('comment-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      hideError($('comments-error'));
      $('comment-submit').disabled = true;

      try {
        const c = await api.commentNote(note.id, $('comment-text').value);
        $('comments').append(node(c));
        $('comment-text').value = '';
        autosize($('comment-text'));
        total++;
        updateTitle();
      } catch (e) {
        showError($('comments-error'), e);
      } finally {
        $('comment-submit').disabled = false;
      }
    });
  }

  updateTitle();

  return loadMore();
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { note, author } = await api.note(noteID);

  document.title = `${note.title} | Freenet`;
  $('title').replaceChildren(...notesTitle(author, note.title));
  $('note').replaceChildren(noteNode(note, author, {
    whole: true,
    onDelete: (n) => confirmDelete(n, $('error'), () => location.assign(notesPath(author))),
  }));
  $('content').hidden = false;

  await initComments(note, me);
  // "Комментарии (N)" of the list leads here.
  if (location.hash === '#comments') $('comments-title').scrollIntoView();
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Заметка не найдена' : 'Ошибка';
  showError($('error'), e.status === 404 ? 'Заметка удалена или скрыта от вас.' : e);
}
