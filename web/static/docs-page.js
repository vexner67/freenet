// "Мои Документы" (/docs), as openvk's: "У Вас N документов", the
// documents newest first, each with "Редактировать" (its name, in
// openvk's "Редактирование документа") and "Удалить", and "Загрузить
// документ".

import { api, currentUser } from './api.js';
import { haveDocuments } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { documentNode, uploadFile } from './docs.js';

let cursor = '';
let total = 0;

function showSummary() {
  $('summary').textContent = haveDocuments(total);
  $('summary').hidden = total === 0;
  $('empty').hidden = total !== 0;
}

// shown is a document of the list with its links; it stands in its own
// place when renamed.
function shown(doc) {
  const node = documentNode(doc, {
    tools: [
      el('a', { textContent: 'Редактировать', onclick: () => openRename(doc, node) }),
      el('a', { textContent: 'Удалить', onclick: () => confirmDelete(doc, node) }),
    ],
  });

  return node;
}

function openRename(doc, node) {
  const modal = openModal('Редактирование документа', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const name = el('input', { type: 'text', maxLength: 128, value: doc.name, ariaLabel: 'Название' });
  const save = button('Сохранить', null, { type: 'submit' });

  modal.body.append(error, el('p', {}, el('b', { textContent: 'Название' })), name);
  modal.actions.append(save, button('Отмена', modal.close, { secondary: true }));
  name.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    save.disabled = true;

    try {
      node.replaceWith(shown(await api.renameDocument(doc.id, name.value)));
      modal.close();
    } catch (e) {
      showError(error, e);
      save.disabled = false;
    }
  });
}

// confirmDelete asks first: the document disappears from the posts and
// messages that attach it too.
function confirmDelete(doc, node) {
  const modal = openModal('Удалить документ');
  modal.body.append(el('p', { textContent: `Документ «${doc.name}» пропадёт и из записей и сообщений, к которым он прикреплён. Удалить?` }));
  modal.actions.append(
    button('Удалить', async () => {
      modal.close();
      hideError($('error'));

      try {
        await api.deleteDocument(doc.id);
        node.remove();
        total--;
        showSummary();
      } catch (e) {
        showError($('error'), e);
      }
    }),
    button('Отмена', modal.close, { secondary: true }),
  );
}

async function loadMore() {
  $('more').hidden = true;

  try {
    const page = await api.documents(cursor);
    total = page.total;
    $('docs').append(...page.documents.map(shown));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    showSummary();
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  $('upload').addEventListener('click', () => uploadFile($('error'), (doc) => {
    $('docs').prepend(shown(doc));
    total++;
    showSummary();
  }));
  $('more').addEventListener('click', loadMore);
  loadMore();
}
