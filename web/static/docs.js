// Documents, as openvk's "Мои Документы": a document is a row with its
// format in a grey box, its name and "дата, размер"; the name downloads
// it. Posts and messages attach them ("Прикрепить документ"), picked from
// one's documents or uploaded there and then.

import { api } from './api.js';
import { DOCUMENT_FORMATS, MAX_DOCUMENTS, MAX_DOCUMENT_BYTES, documentFormat, fileSize, formatPostDate } from './labels.js';
import { el, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// ACCEPT is what the file picker offers: the formats the server takes.
export const ACCEPT = DOCUMENT_FORMATS.map((f) => '.' + f).join(',');

// documentNode is a document as a row; tools are links at its right end,
// as "Редактировать" of one's own. A click on the row picks it when pick
// is given, and downloads it otherwise.
export function documentNode(doc, { tools = [], pick = null } = {}) {
  const name = pick
    ? el('a', { className: 'doc-name', textContent: doc.name, onclick: () => pick(doc) })
    : el('a', { className: 'doc-name', href: doc.url, download: doc.name, textContent: doc.name });

  return el('div', { className: 'doc-item' },
    el('a', { className: 'doc-icon', href: doc.url, download: doc.name, textContent: doc.ext }),
    el('div', { className: 'doc-content' },
      name,
      el('div', { className: 'doc-info', textContent: `${formatPostDate(doc.createdAt)}, ${fileSize(doc.size)}` }),
    ),
    tools.length ? el('div', { className: 'doc-tools' }, ...tools) : null,
  );
}

// documentsNode is the documents a post or a message attaches, under its
// words and photos; null for none.
export function documentsNode(docs) {
  if (!docs?.length) return null;

  return el('div', { className: 'doc-list' }, ...docs.map((d) => documentNode(d)));
}

// checkFile tells why a file is not taken before it is sent; '' when it is.
export function checkFile(file) {
  if (!DOCUMENT_FORMATS.includes(documentFormat(file.name))) return tr('Формат файла не разрешён.');
  if (file.size > MAX_DOCUMENT_BYTES) return tr('Файл слишком большой: не больше 10 МБ.');
  if (file.size === 0) return tr('Файл пустой.');

  return '';
}

// uploadFile is openvk's "Загрузка документа" (showDocumentUploadDialog
// in Web/static/js/al_docs.js): the limits first, then "Выбрать файл";
// the chosen file is uploaded as a document and onDone gets it; a file
// dropped on the button is taken too.
export function uploadFile(onDone) {
  const modal = openModal(tr('Загрузка документа'));
  const error = el('div', { className: 'error', hidden: true });
  const input = el('input', { type: 'file', accept: ACCEPT, hidden: true });
  const choose = button(tr('Выбрать файл'), () => input.click());
  const frame = el('div', { className: 'doc-upload-frame' }, choose, input);
  // openvk shows ten of the formats, picked anew each time.
  const formats = [...DOCUMENT_FORMATS].sort(() => Math.random() - 0.59).slice(0, 10).join(', ');

  modal.box.classList.add('doc-upload');
  modal.body.append(error,
    el('b', { textContent: tr('Ограничения') }),
    el('ul', { className: 'doc-upload-limits' },
      el('li', { textContent: tr('Файл не должен превышать {0} МБ.', MAX_DOCUMENT_BYTES >> 20) }),
      el('li', { textContent: tr('Разрешены следующие типы файлов: {0}.', formats) }),
      el('li', { textContent: tr('Файл не должен нарушать авторские права и правила сайта.') })),
    frame);
  modal.actions.append(button(tr('Закрыть'), modal.close, { secondary: true }));

  async function take(file) {
    if (!file) return;
    hideError(error);
    const why = checkFile(file);
    if (why) {
      showError(error, why);
      return;
    }

    choose.disabled = true;
    try {
      const doc = await api.uploadDocument(file);
      modal.close();
      onDone(doc);
    } catch (e) {
      showError(error, e);
      choose.disabled = false;
    }
  }

  input.addEventListener('change', () => {
    const file = input.files[0];
    input.value = '';
    take(file);
  });
  frame.addEventListener('dragover', (e) => e.preventDefault());
  frame.addEventListener('drop', (e) => {
    e.preventDefault();
    take(e.dataTransfer.files[0]);
  });
}

// openPicker is openvk's "Выбор документа": one's documents, newest
// first, and "Загрузить документ"; a click attaches one.
function openPicker(onPick) {
  const modal = openModal(tr('Выбор документа'));
  const error = el('div', { className: 'error', hidden: true });
  const list = el('div', { className: 'doc-picker' });
  const more = el('a', { className: 'wall-more', textContent: tr('Показать ещё'), hidden: true });
  let cursor = '';

  const pick = (doc) => {
    modal.close();
    onPick(doc);
  };

  async function load() {
    more.hidden = true;
    try {
      const page = await api.documents(cursor);
      cursor = page.nextCursor;
      list.append(...page.documents.map((d) => documentNode(d, { pick })));
      if (!list.childElementCount) list.append(el('p', { className: 'hint', textContent: tr('Документов нет') }));
      more.hidden = !cursor;
    } catch (e) {
      showError(error, e);
    }
  }

  more.addEventListener('click', load);
  modal.body.append(error, list, more);
  modal.actions.append(
    button(tr('Загрузить документ'), () => uploadFile(pick)),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
  load();
}

// initDocuments wires "Прикрепить документ" of a form: link opens the
// picker, list shows what is attached, each with a cross; onChange runs
// whenever documents come or go, not when the form is made. upload opens
// "Загрузка документа" at once, as openvk's messenger does: a message
// sends a new file, not one of "Мои Документы".
export function initDocuments({ link, list, onChange = () => {}, upload = false }) {
  let docs = [];

  function render(changed = true) {
    list.replaceChildren(...docs.map((doc) => documentNode(doc, {
      tools: [el('a', { className: 'doc-remove', title: tr('Не прикреплять'), textContent: '×', onclick: () => remove(doc) })],
    })));
    list.hidden = docs.length === 0;
    link.hidden = docs.length >= MAX_DOCUMENTS;
    if (changed) onChange();
  }

  function remove(doc) {
    docs = docs.filter((d) => d !== doc);
    render();
  }

  const add = (doc) => {
    if (!docs.some((d) => d.id === doc.id) && docs.length < MAX_DOCUMENTS) docs.push(doc);
    render();
  };
  link.addEventListener('click', () => (upload ? uploadFile(add) : openPicker(add)));

  render(false);

  return {
    ids: () => docs.map((d) => d.id),
    count: () => docs.length,
    clear() {
      docs = [];
      render();
    },
  };
}
