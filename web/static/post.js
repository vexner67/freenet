// The page of a post at /post/<id>: the post, its comments oldest first and
// the form for a new comment, as on openvk.

import { api, currentUser } from './api.js';
import { commentNode } from './comments.js';
import { $, el, fullName, renderChrome, showError, hideError } from './layout.js';
import { postNode } from './posts.js';

const COUNTER_FROM = 900;
const MAX_LENGTH = 1000;

const postID = decodeURIComponent(location.pathname.slice('/post/'.length));

function renderTitle(post) {
  const author = '/' + post.author.nickname;
  const name = fullName(post.author);

  document.title = `Запись ${name} | Freenet`;
  $('title').replaceChildren(
    el('a', { href: author, textContent: name }),
    ' » ',
    el('a', { href: author, textContent: 'Стена' }),
    ' » Запись',
  );
}

function initComments(post, me) {
  const list = $('comments');
  const error = $('comments-error');

  let total = 0;
  let cursor = '';
  // Comments sent from this page go last; pages loaded later skip them.
  const sent = new Set();

  function updateCount() {
    $('comments-count').textContent = total > 0 ? `(${total})` : '';
    $('comments-empty').hidden = total > 0;
  }

  // The author of a comment and the author of the post may delete it.
  const canDelete = (comment) => Boolean(me) && (me.id === comment.author.id || me.id === post.author.id);
  const commentNodeFor = (comment) => commentNode(comment, canDelete(comment) && remove);

  async function remove(comment, node) {
    hideError(error);

    try {
      await api.deleteComment(comment.id);
    } catch (e) {
      showError(error, e);
      return;
    }

    node.replaceWith(el('div', { className: 'post-deleted', textContent: 'Комментарий удалён.' }));
    total--;
    updateCount();
  }

  async function loadMore() {
    hideError(error);

    try {
      const page = await api.comments(post.id, cursor);
      const fresh = page.comments.filter((c) => !sent.has(c.id));
      const firstSent = list.querySelector('[data-sent]');

      for (const comment of fresh) list.insertBefore(commentNodeFor(comment), firstSent);

      total = page.total;
      cursor = page.nextCursor;
      $('comments-more').hidden = !cursor;
      updateCount();
    } catch (e) {
      showError(error, e);
    }
  }

  function updateCounter() {
    const left = MAX_LENGTH - $('comment-text').value.length;
    $('comment-counter').textContent = left <= MAX_LENGTH - COUNTER_FROM ? `осталось ${left}` : '';
  }

  async function submit(event) {
    event.preventDefault();
    hideError(error);

    const button = $('comment-submit');
    button.disabled = true;

    try {
      const comment = await api.createComment(post.id, $('comment-text').value);
      const node = commentNodeFor(comment);

      node.dataset.sent = '';
      sent.add(comment.id);
      list.append(node);
      $('comment-text').value = '';
      total++;
      updateCount();
      updateCounter();
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  }

  if (me?.profileCompleted) {
    const form = $('comment-form');
    const text = $('comment-text');

    form.hidden = false;
    form.addEventListener('submit', submit);
    text.addEventListener('input', updateCounter);
    // Ctrl+Enter (Cmd+Enter on a Mac) sends, as on the old sites.
    text.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) form.requestSubmit();
    });
  } else {
    const hint = $('comment-hint');
    hint.replaceChildren(...(me
      ? ['Чтобы оставить комментарий, ', el('a', { href: '/edit', textContent: 'заполните свою страницу' }), '.']
      : ['Чтобы оставить комментарий, ', el('a', { href: '/', textContent: 'войдите на сайт' }), '.']));
    hint.hidden = false;
  }

  $('comments-more').addEventListener('click', loadMore);

  return loadMore();
}

async function removePost(post) {
  hideError($('error'));

  try {
    await api.deletePost(post.id);
    location.href = '/' + post.author.nickname;
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const post = await api.post(postID);

  renderTitle(post);
  $('post').append(postNode(post, {
    onDelete: me?.id === post.author.id && removePost,
    canLike: Boolean(me),
    onError: (e) => showError($('error'), e),
    withCommentsLink: false,
  }));
  $('content').hidden = false;

  await initComments(post, me);
} catch (e) {
  const notFound = e.status === 404 || e.status === 400;
  $('title').textContent = notFound ? 'Запись не найдена' : 'Ошибка';
  document.title = $('title').textContent + ' | Freenet';
  showError($('error'), notFound ? 'Запись удалена или никогда не существовала.' : e);
}
