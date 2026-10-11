// "Заметки" of a person, as openvk's: "Мои Заметки" for your own, "Анна »
// Заметки" otherwise; how many were found with "Добавить запись" for
// yourself, and the notes, newest first. /notes is yours,
// /notes?u=<nickname> someone's.

import { api, currentUser } from './api.js';
import { foundNotes } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { confirmDelete, noteNode } from './notes.js';

const nickname = new URLSearchParams(location.search).get('u') ?? '';

let cursor = '';
let total = 0;

function renderHead(page, mine) {
  const { author } = page;

  document.title = `Заметки ${fullName(author)} | Freenet`;
  if (mine) $('title').textContent = 'Мои Заметки';
  else $('title').replaceChildren(el('a', { href: pagePath(author), textContent: fullName(author) }), ' » Заметки');

  if (mine) $('summary-links').replaceChildren(' | ', el('a', { href: '/notes/create', textContent: 'Добавить запись' }));
  $('summary').hidden = false;
}

function showCount(mine) {
  $('summary-count').textContent = foundNotes(total);
  // openvk greets the owner of no notes.
  $('welcome').hidden = !mine || total > 0;
  $('empty')?.remove();
  if (!mine && total === 0) $('notes').append(el('p', { className: 'hint', id: 'empty', textContent: 'Тут ничего нет... Пока...' }));
}

async function loadMore(owner, me) {
  hideError($('error'));

  try {
    const page = await api.notes(owner, cursor);
    const mine = page.author.id === me?.id;

    if (!cursor) {
      total = page.total;
      renderHead(page, mine);
      showCount(mine);
    }

    $('notes').append(...page.notes.map((note) => noteNode(note, page.author, {
      onDelete: (n, node) => confirmDelete(n, $('error'), () => {
        node.replaceWith(el('div', { className: 'post-deleted', textContent: 'Заметка удалена.' }));
        total--;
        showCount(mine);
      }),
    })));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    if (!cursor) $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Заметки';
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!nickname && !me) {
  location.replace('/');
} else if (!nickname && !me.profileCompleted) {
  $('title').textContent = 'Мои Заметки';
  showError($('error'), 'Сначала заполните свою страницу.');
} else {
  const owner = nickname || me.nickname;

  $('more').addEventListener('click', () => loadMore(owner, me));
  await loadMore(owner, me);
}
