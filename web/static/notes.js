// What the pages of notes ("Заметки") share: their paths, the title "Анна
// » Заметки » …" of openvk, a note as the list and its own page show it,
// and the block "Заметки" of a person's page.

import { api } from './api.js';
import { formatPostDate, notesCount } from './labels.js';
import { avatarURL, el, fullName, pagePath, showError, textNode } from './layout.js';
import { button, openModal } from './modal.js';
import { separated } from './posts.js';

// notesPath is the list of notes of author, a person.
export function notesPath(author) {
  return '/notes?u=' + encodeURIComponent(author.nickname);
}

export function notePath(note) {
  return '/note/' + encodeURIComponent(note.id);
}

// notesTitle is "<author> » Заметки » <what>…", the author and the notes
// as links; what are strings or nodes.
export function notesTitle(author, ...what) {
  return [
    el('a', { href: pagePath(author), textContent: fullName(author) }),
    ' » ',
    el('a', { href: notesPath(author), textContent: 'Заметки' }),
    ...what.flatMap((w) => [' » ', w]),
  ];
}

// commentsLabel is openvk's "Комментарии (3)" or "Нет комментариев".
export function commentsLabel(n) {
  return n > 0 ? `Комментарии (${n})` : 'Нет комментариев';
}

// confirmDelete asks before a note goes with its comments (openvk deletes
// it at once), then calls onDeleted; a failure shows in error.
export function confirmDelete(note, error, onDeleted) {
  const modal = openModal('Удалить заметку');
  modal.body.textContent = `Вы уверены, что хотите удалить заметку «${note.title}»?`;
  modal.actions.append(
    button('Удалить', async () => {
      try {
        await api.deleteNote(note.id);
        modal.close();
        onDeleted();
      } catch (e) {
        modal.close();
        showError(error, e);
      }
    }),
    button('Отмена', modal.close, { secondary: true }),
  );
}

// noteNode is a note as openvk's list shows it: the photo of the author,
// the title, who wrote it and when, the text — whole on the note's own
// page, its beginning with "Показать больше" in the list — and below
// "Комментарии (N)", "Удалить" and "Редактировать" for the author.
// onDelete(note, node) is called for "Удалить".
export function noteNode(note, author, { whole = false, onDelete } = {}) {
  const path = notePath(note);

  const byline = [el('a', { href: pagePath(author), textContent: fullName(author) }), ' ' + formatPostDate(note.createdAt)];
  if (note.editedAt) byline.push(` (ред. ${formatPostDate(note.editedAt)})`);

  const menu = [];
  if (!whole) menu.push(el('a', { href: path + '#comments', textContent: commentsLabel(note.comments) }));
  if (note.mine) {
    menu.push(
      el('a', { textContent: 'Удалить', onclick: () => onDelete(note, node) }),
      el('a', { href: path + '/edit', textContent: 'Редактировать' }),
    );
  }

  const node = el('div', { className: 'post note-item' },
    el('a', { className: 'post-avatar', href: pagePath(author) }, el('img', { src: avatarURL(author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'note-title' }, whole ? el('b', { textContent: note.title }) : el('a', { href: path }, el('b', { textContent: note.title }))),
      el('div', { className: 'hint note-byline' }, ...byline),
      textNode(note.text, { whole, rest: note.textCut ? () => api.note(note.id).then((page) => page.note.text) : null }),
      menu.length > 0 ? el('div', { className: 'post-menu' }, el('span', {}, ...separated(menu))) : null,
    ),
  );

  return node;
}

// notesBlock is the block "Заметки" of a person's page, as openvk's: how
// many, "Все" and the titles of the newest with their dates.
export function notesBlock(page) {
  return [
    el('div', { className: 'section-title', textContent: 'Заметки' }),
    el('div', { className: 'subtitle' },
      el('span', { textContent: notesCount(page.total) }),
      el('a', { href: notesPath(page.author), textContent: 'Все' })),
    el('div', { className: 'side-text side-notes' }, ...page.notes.map((note) => el('div', { className: 'side-note' },
      el('a', { href: notePath(note) }, el('b', { textContent: note.title })),
      el('div', { className: 'hint', textContent: formatPostDate(note.createdAt) }),
    ))),
  ];
}
