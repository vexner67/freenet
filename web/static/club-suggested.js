// The posts suggested for a club ("Предложка"), as openvk's page: for its
// admins all of them, "Предложенные записи", each with "Опубликовать
// запись" and "Отклонить"; with ?type=my those you suggested,
// "Предложенные вами записи", which you may delete.

import { api, currentUser } from './api.js';
import { formatPostDate, suggestedInClub, wrote } from './labels.js';
import { $, avatarURL, clubRef, el, fullName, pagePath, renderChrome, showError, hideError, autosize, mentionNodes } from './layout.js';
import { button, openModal } from './modal.js';
import { photosNode, postPath } from './posts.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/suggested'.length));
const mine = new URLSearchParams(location.search).get('type') === 'my';

let club;
let cursor = '';
let total = 0;
// handled: some were published, declined or deleted here; once all are,
// the head says so instead of "nothing suggested yet".
let handled = false;

function updateCount() {
  $('count').textContent = suggestedInClub(total, mine);
  $('count').hidden = total === 0 && !handled;
  $('empty').textContent = mine ? 'Вы ещё не предлагали записей в эту группу.' : 'В эту группу ещё не предлагали записей.';
  $('empty').hidden = total > 0 || handled;
}

// suggestionNode is a post as openvk shows a suggested one: no likes,
// comments or sharing; the buttons for the admins, "Удалить" for its
// author.
function suggestionNode(s) {
  const path = pagePath(s.author);
  const node = el('div', { className: 'post' },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(s.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(s.author) }),
        ' ' + wrote(s.author.gender),
        el('div', { className: 'post-date', textContent: formatPostDate(s.createdAt) }),
      ),
      el('div', { className: 'post-content' },
        s.text ? el('div', { className: 'post-text' }, ...mentionNodes(s.text)) : '',
        photosNode(s.photos) ?? '',
      ),
      mine
        ? el('div', { className: 'post-menu' }, el('span', {}, el('a', { textContent: 'Удалить', onclick: () => remove(s, node) })))
        : el('div', { className: 'suggestion-controls' },
          button('Опубликовать запись', () => openPublish(s, node)),
          button('Отклонить', () => remove(s, node), { secondary: true }),
        ),
    ),
  );

  return node;
}

// openPublish is openvk's "Публикация предложенной записи": the text to
// leave or change and "Подпись автора", on by default.
function openPublish(s, node) {
  const modal = openModal('Публикация предложенной записи');
  const text = el('textarea', { className: 'publish-text', maxLength: 60000, value: s.text });
  const signed = el('input', { type: 'checkbox', checked: true });
  const error = el('div', { className: 'error', hidden: true });

  modal.body.append(text, el('label', { className: 'publish-signed' }, signed, ' Подпись автора'), error);
  autosize(text);
  modal.actions.append(
    button('Опубликовать', async (event) => {
      event.target.disabled = true;
      hideError(error);

      try {
        const post = await api.publishSuggestion(s.id, text.value, signed.checked);
        modal.close();
        gone(node);
        published(post);
      } catch (e) {
        showError(error, e);
        event.target.disabled = false;
      }
    }),
    button('Отмена', modal.close, { secondary: true }),
  );
  text.focus();
}

// published is openvk's "Запись успешно опубликована" with the way to it.
function published(post) {
  const link = post ? el('a', { href: postPath(post), textContent: 'Перейти к ней' }) : '';
  $('notice').replaceChildren('Запись успешно опубликована. ', link);
  $('notice').hidden = false;
}

async function remove(s, node) {
  hideError($('error'));

  try {
    await api.deleteSuggestion(s.id);
    gone(node);
  } catch (e) {
    showError($('error'), e);
  }
}

function gone(node) {
  node.remove();
  handled = true;
  total--;
  updateCount();
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.clubSuggestions(ref, { mine, cursor });

    total = page.total;
    $('posts').append(...page.suggestions.map((s) => suggestionNode(s)));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    updateCount();
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    club = await api.club(ref);
    const path = '/club/' + encodeURIComponent(clubRef(club));
    const heading = mine ? 'Предложенные вами записи' : 'Предложенные записи';

    document.title = `${heading}: ${club.name} | Freenet`;
    $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ` » ${heading}`);
    $('more').addEventListener('click', loadMore);
    await loadMore();
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
    showError($('error'), e);
  }
}
