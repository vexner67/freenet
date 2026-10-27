// The editor of an article at /article/new and /article/<id>/edit, as
// VK's editor of "Статьи": a cover, a title and the text, written in place
// with a bar of tools — bold, italic, struck out, a link, a heading and a
// subheading, a quote, lists, code, a divider and photos with captions. A
// new article goes on the author's wall as a post that links to it,
// unless they untick "Опубликовать на моей стене".

import { api, currentUser, started } from './api.js';
import { articlePath, articleURL, blockNodes, photoFigure, readBlocks } from './articles.js';
import { $, el, isPhone, profilePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// The text of an article holds up to 100 000 characters; the counter shows
// near the end.
const MAX_LENGTH = 100000;
const COUNTER_FROM = MAX_LENGTH - 5000;
const MAX_PHOTOS = 30;
const DRAFT_KEY = 'freenet.article-draft';

const editedID = /^\/article\/([^/]+)\/edit$/.exec(location.pathname)?.[1] ?? null;

const body = $('body-input');
const error = $('error');
let cover = null;
let dirty = false;

// tools are the buttons of the bar: what each does and how it tells it
// is on where the caret stands.
const tools = [
  { name: 'bold', run: () => exec('bold'), on: () => document.queryCommandState('bold') },
  { name: 'italic', run: () => exec('italic'), on: () => document.queryCommandState('italic') },
  { name: 'strike', run: () => exec('strikeThrough'), on: () => document.queryCommandState('strikeThrough') },
  { name: 'link', run: () => askLink(), on: () => Boolean(closest('a')) },
  null,
  { name: 'heading', run: () => toggleBlock('h2'), on: () => blockTag() === 'h2' },
  { name: 'subheading', run: () => toggleBlock('h3'), on: () => blockTag() === 'h3' },
  { name: 'quote', run: () => toggleBlock('blockquote'), on: () => Boolean(closest('blockquote')) },
  { name: 'code', run: () => toggleBlock('pre'), on: () => blockTag() === 'pre' },
  { name: 'bullets', run: () => exec('insertUnorderedList'), on: () => Boolean(closest('ul')) },
  { name: 'numbers', run: () => exec('insertOrderedList'), on: () => Boolean(closest('ol')) },
  null,
  { name: 'divider', run: () => insertBlock(el('hr')), on: () => false },
  { name: 'photo', run: () => $('photo-file').click(), on: () => false },
];

const toolTitles = {
  bold: tr('Жирный'),
  italic: tr('Курсив'),
  strike: tr('Зачёркнутый'),
  link: tr('Ссылка'),
  heading: tr('Заголовок'),
  subheading: tr('Подзаголовок'),
  quote: tr('Цитата'),
  code: tr('Код'),
  bullets: tr('Маркированный список'),
  numbers: tr('Нумерованный список'),
  divider: tr('Разделитель'),
  photo: tr('Фотография'),
};

function exec(command, value = null) {
  body.focus();
  document.execCommand(command, false, value);
  changed();
}

// selectionIn is the range of the caret, if it stands in the text.
function selectionIn() {
  const selection = getSelection();
  if (!selection.rangeCount) return null;
  const range = selection.getRangeAt(0);

  return body.contains(range.commonAncestorContainer) ? range : null;
}

function closest(selector) {
  const range = selectionIn();
  if (!range) return null;
  const node = range.startContainer.nodeType === Node.ELEMENT_NODE ? range.startContainer : range.startContainer.parentElement;
  const found = node?.closest(selector);

  return found && body.contains(found) ? found : null;
}

// topBlock is the block of the text the caret stands in: a child of the
// text itself.
function topBlock() {
  const range = selectionIn();
  let node = range?.startContainer;
  while (node && node.parentNode !== body) node = node.parentNode;

  return node ?? null;
}

function blockTag() {
  return closest('h1, h2, h3, h4, h5, h6, pre, p, li, blockquote')?.tagName.toLowerCase() ?? '';
}

// toggleBlock makes the block of the caret a heading, a quote or code, or
// a paragraph again when it is one already.
function toggleBlock(tag) {
  const on = tag === 'blockquote' ? Boolean(closest('blockquote')) : blockTag() === tag;
  exec('formatBlock', on ? 'p' : tag);
  if (on && tag === 'blockquote') {
    // A browser leaves the paragraph in the quote: take it out.
    const quote = closest('blockquote');
    if (quote) quote.replaceWith(...quote.childNodes);
  }
}

// insertBlock puts a divider or a photo after the block of the caret, and
// an empty paragraph after it, if it ends the text, to write on.
function insertBlock(node) {
  const after = topBlock();
  if (after) after.after(node);
  else body.append(node);

  if (!node.nextSibling) node.after(el('p', {}, el('br')));
  const next = node.nextSibling;
  if (next.nodeType === Node.ELEMENT_NODE && next.isContentEditable) {
    const range = document.createRange();
    range.setStart(next, 0);
    range.collapse(true);
    getSelection().removeAllRanges();
    getSelection().addRange(range);
  }
  changed();
}

// webAddress is what was typed as a link: an http(s) address, with
// https:// added to one written without it; '' for anything else.
function webAddress(text) {
  const typed = text.trim();
  if (!typed) return '';
  const address = /^[a-z][a-z0-9+.-]*:/i.test(typed) ? typed : 'https://' + typed;
  try {
    const url = new URL(address);
    return (url.protocol === 'http:' || url.protocol === 'https:') && url.host ? url.href : '';
  } catch {
    return '';
  }
}

// askLink makes the selected words a link, or takes the link away from
// those that are one.
function askLink() {
  const range = selectionIn();
  if (!range) return;
  const link = closest('a');
  const saved = range.cloneRange();

  const modal = openModal(tr('Ссылка'), { tag: 'form' });
  const fail = el('div', { className: 'error', hidden: true });
  const input = el('input', { type: 'text', inputMode: 'url', value: link?.getAttribute('href') ?? '', placeholder: 'https://', ariaLabel: tr('Адрес ссылки') });
  modal.body.append(fail, el('p', {}, el('b', { textContent: tr('Адрес ссылки') })), input);
  modal.actions.append(button(tr('Сохранить'), null, { type: 'submit' }));
  if (link) {
    modal.actions.append(button(tr('Убрать ссылку'), () => {
      modal.close();
      restore(saved);
      const whole = document.createRange();
      whole.selectNodeContents(link);
      restore(whole);
      exec('unlink');
    }, { secondary: true }));
  }
  modal.actions.append(button(tr('Отмена'), modal.close, { secondary: true }));
  input.focus();

  modal.box.addEventListener('submit', (event) => {
    event.preventDefault();
    const address = webAddress(input.value);
    if (!address) {
      showError(fail, tr('Ссылка должна вести на сайт: http:// или https://.'));
      return;
    }

    modal.close();
    if (link) {
      link.href = address;
      changed();
    } else if (saved.collapsed) {
      // Where nothing is selected, the address itself is the link.
      const node = el('a', { href: address, textContent: address });
      saved.insertNode(node);
      const after = document.createRange();
      after.setStartAfter(node);
      restore(after);
      changed();
    } else {
      restore(saved);
      exec('createLink', address);
    }
  });
}

function restore(range) {
  body.focus();
  getSelection().removeAllRanges();
  getSelection().addRange(range);
}

function photosCount() {
  return body.querySelectorAll('figure[data-photo]').length + (cover ? 1 : 0);
}

async function uploadPhoto(file) {
  hideError(error);
  if (photosCount() >= MAX_PHOTOS) {
    showError(error, tr('В статье может быть не больше 30 фотографий.'));
    return null;
  }

  try {
    return await api.uploadPhoto(file);
  } catch (e) {
    showError(error, e);
    return null;
  }
}

function showCover() {
  const box = $('cover');
  box.classList.toggle('with-cover', Boolean(cover));
  box.querySelector('img')?.remove();
  if (cover) box.prepend(el('img', { src: cover.large, alt: '' }));
  $('cover-add').textContent = cover ? tr('Сменить обложку') : tr('Добавить обложку');
  $('cover-remove').hidden = !cover;
}

function initCover() {
  $('cover-add').addEventListener('click', () => $('cover-file').click());
  $('cover-file').addEventListener('change', async () => {
    const file = $('cover-file').files[0];
    $('cover-file').value = '';
    if (!file) return;

    const old = cover;
    cover = null;
    const photo = await uploadPhoto(file);
    cover = photo ?? old;
    showCover();
    changed();
  });
  $('cover-remove').addEventListener('click', () => {
    cover = null;
    showCover();
    changed();
  });
}

function initToolbar() {
  const buttons = tools.map((tool) => {
    if (!tool) return el('span', { className: 'article-tool-gap' });

    const node = el('button', { type: 'button', className: `article-tool tool-${tool.name}`, title: toolTitles[tool.name], ariaLabel: toolTitles[tool.name] });
    // The text keeps the caret and the selection.
    node.addEventListener('mousedown', (event) => event.preventDefault());
    node.addEventListener('click', () => {
      tool.run();
      showActive();
    });
    tool.node = node;
    return node;
  });
  $('toolbar').replaceChildren(...buttons);

  $('photo-file').addEventListener('change', async () => {
    const file = $('photo-file').files[0];
    $('photo-file').value = '';
    if (!file) return;

    const photo = await uploadPhoto(file);
    if (photo) insertBlock(photoFigure(photo, '', true));
  });
}

function showActive() {
  const inside = Boolean(selectionIn());
  for (const tool of tools) tool?.node.classList.toggle('active', inside && tool.on());
}

function textLength() {
  return readBlocks(body).reduce((n, b) => n + (b.spans ?? []).reduce((m, s) => m + s.text.length, 0) + (b.caption?.length ?? 0), 0);
}

function changed() {
  dirty = true;
  const n = textLength();
  $('counter').textContent = n >= COUNTER_FROM ? `${n} / ${MAX_LENGTH}` : '';
  $('counter').classList.toggle('over', n > MAX_LENGTH);
  if (!editedID) saveDraft();
}

// The draft of a new article stays in the browser until it is published:
// its title and text, without photos, which are not kept until then.
function saveDraft() {
  try {
    const blocks = readBlocks(body).filter((b) => b.kind !== 'photo');
    localStorage.setItem(DRAFT_KEY, JSON.stringify({ title: $('title-input').value, blocks }));
  } catch {
    // Private mode: the draft lasts as long as the page.
  }
}

function loadDraft() {
  try {
    return JSON.parse(localStorage.getItem(DRAFT_KEY) ?? 'null');
  } catch {
    return null;
  }
}

function dropDraft() {
  try {
    localStorage.removeItem(DRAFT_KEY);
  } catch {
    // Nothing kept.
  }
}

function fill(article) {
  $('title-input').value = article?.title ?? '';
  cover = article?.cover ?? null;
  showCover();
  const nodes = blockNodes(article?.blocks ?? [], true);
  body.replaceChildren(...(nodes.length ? nodes : [el('p', {}, el('br'))]));
}

function initBody() {
  document.execCommand('defaultParagraphSeparator', false, 'p');

  body.addEventListener('input', changed);
  // Only words come in: a page pasted keeps none of its looks or scripts.
  body.addEventListener('paste', (event) => {
    if (event.target.closest?.('.article-caption')) return;
    event.preventDefault();
    const text = event.clipboardData.getData('text/plain');
    if (text) document.execCommand('insertText', false, text);
  });
  body.addEventListener('drop', (event) => event.preventDefault());
  // Enter in a caption ends it; the caption writes no lines.
  body.addEventListener('keydown', (event) => {
    if (event.target.closest?.('.article-caption') && event.key === 'Enter') event.preventDefault();
  });
  body.addEventListener('input', (event) => {
    if (event.target.closest?.('.article-caption')) changed();
  }, true);
  // An emptied text keeps a paragraph to write in.
  body.addEventListener('keyup', () => {
    if (!body.firstChild) body.append(el('p', {}, el('br')));
  });
  document.addEventListener('selectionchange', showActive);

  $('title-input').addEventListener('input', changed);
  // Enter after the title goes on to the text.
  $('title-input').addEventListener('keydown', (event) => {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    body.focus();
  });
}

async function publish(event, me) {
  event.preventDefault();
  hideError(error);
  $('publish').disabled = true;

  const draft = {
    title: $('title-input').value,
    cover: cover?.id ?? '',
    blocks: readBlocks(body).map((b) => (b.kind === 'photo' ? { ...b, photo: b.photo.id } : b)),
  };

  try {
    const article = editedID ? await api.editArticle(editedID, draft) : await api.createArticle(draft);
    if (!editedID) {
      dropDraft();
      if ($('to-wall').checked) await api.createPost(me.nickname, articleURL(article.id)).catch(() => null);
    }
    dirty = false;
    location.href = articlePath(article.id);
  } catch (e) {
    showError(error, e);
    $('publish').disabled = false;
  }
}

const wanted = editedID ? started(api.article(decodeURIComponent(editedID))) : null;
const me = await currentUser().catch(() => null);
renderChrome(me);

async function open() {
  if (!me) {
    location.href = '/';
    return;
  }
  if (!me.profileCompleted) {
    showError(error, tr('Заполните профиль, прежде чем писать статьи.'));
    return;
  }

  if (editedID) {
    const article = await wanted;
    if (article.author.id !== me.id) {
      showError(error, tr('Изменить статью может только её автор.'));
      return;
    }

    document.title = tr('Редактирование статьи') + ' | Freenet';
    $('title').textContent = tr('Редактирование статьи');
    $('publish').textContent = tr('Сохранить');
    $('to-wall-label').hidden = true;
    fill(article);
  } else {
    fill(loadDraft());
  }

  initCover();
  initToolbar();
  initBody();
  $('editor-form').addEventListener('submit', (event) => publish(event, me));
  $('cancel').addEventListener('click', () => {
    location.href = editedID ? articlePath(decodeURIComponent(editedID)) : profilePath(me);
  });
  addEventListener('beforeunload', (event) => {
    if (dirty && editedID) event.preventDefault();
  });
  $('editor-form').hidden = false;
  $('editor-form').classList.toggle('phone', isPhone());
  if (!$('title-input').value) $('title-input').focus();
}

try {
  await open();
} catch (e) {
  const notFound = e.status === 404 || e.status === 400;
  $('title').textContent = notFound ? tr('Статья не найдена') : tr('Ошибка');
  showError(error, notFound ? tr('Статья удалена или никогда не существовала.') : e);
}
