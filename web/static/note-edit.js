// openvk's "Добавить запись" at /notes/create and "Редактировать заметку"
// at /note/<id>/edit: "Название", "Содержание" and "Сохранить", which
// opens the note. The text is plain: openvk takes HTML there.

import { api, ApiError, currentUser } from './api.js';
import { $, autosize, renderChrome, showError, hideError } from './layout.js';
import { notePath, notesTitle } from './notes.js';

const creating = location.pathname === '/notes/create';
const noteID = creating ? '' : decodeURIComponent(location.pathname.slice('/note/'.length, -'/edit'.length));

function initForm(save) {
  $('form').hidden = false;
  autosize($('note-text'));
  $('note-title').focus();

  $('form').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError($('error'));
    $('submit').disabled = true;

    try {
      location.assign(notePath(await save($('note-title').value, $('note-text').value)));
    } catch (e) {
      showError($('error'), e);
      $('submit').disabled = false;
    }
  });
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  location.replace('/');
} else if (creating) {
  document.title = 'Добавить запись | Freenet';
  $('title').replaceChildren(...notesTitle(me, 'Добавить запись'));

  if (!me.profileCompleted) {
    $('title').textContent = 'Добавить запись';
    showError($('error'), 'Сначала заполните свою страницу.');
  } else {
    initForm((title, text) => api.createNote(title, text));
  }
} else {
  try {
    const { note, author } = await api.note(noteID);

    document.title = 'Редактировать заметку | Freenet';
    $('title').replaceChildren(...notesTitle(author, 'Редактировать заметку'));
    if (!note.mine) throw new Error('Недостаточно прав.');

    $('note-title').value = note.title;
    $('note-text').value = note.text;
    $('cancel').href = notePath(note);
    $('cancel').hidden = false;
    initForm((title, text) => api.editNote(note.id, title, text));
  } catch (e) {
    if (e.status === 404) $('title').textContent = 'Заметка не найдена';
    showError($('error'), e instanceof ApiError ? e : e.message);
  }
}
