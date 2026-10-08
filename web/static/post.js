// The page of a post at /post/<id>: the post, its comments oldest first and
// the form for a new comment, as on openvk.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { commentNode } from './comments.js';
import { genitiveName } from './labels.js';
import { $, el, fullName, renderChrome, showError, hideError } from './layout.js';
import { postNode } from './posts.js';

const COUNTER_FROM = 900;
const MAX_LENGTH = 1000;

const postID = decodeURIComponent(location.pathname.slice('/post/'.length));

// wallOf is the owner of the wall a post is on.
const wallOf = (post) => post.wall ?? post.author;

// renderTitle is "Владелец » Стена » Запись", as openvk heads a post with
// the owner of its wall.
function renderTitle(post) {
  const owner = wallOf(post);
  const path = '/' + owner.nickname;
  const name = fullName(owner);

  document.title = `Запись на стене ${genitiveName(owner)} | Freenet`;
  $('title').replaceChildren(
    el('a', { href: path, textContent: name }),
    ' » ',
    el('a', { href: path, textContent: 'Стена' }),
    ' » Запись',
  );
}

function initComments(post, me) {
  const list = $('comments');
  const error = $('comments-error');

  let total = 0;
  let cursor = '';
  let attachments;
  // Comments sent from this page go last; pages loaded later skip them.
  const sent = new Set();

  function updateCount() {
    $('comments-count').textContent = total > 0 ? `(${total})` : '';
    $('comments-empty').hidden = total > 0;
  }

  // The author of a comment, the author of the post and the owner of the
  // wall may delete it.
  const canDelete = (comment) => Boolean(me) && [comment.author.id, post.author.id, wallOf(post).id].includes(me.id);
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

    if (attachments.busy()) {
      showError(error, 'Дождитесь, пока загрузятся фотографии.');
      return;
    }

    const button = $('comment-submit');
    button.disabled = true;

    try {
      const comment = await api.createComment(post.id, $('comment-text').value, attachments.ids());
      const node = commentNodeFor(comment);

      node.dataset.sent = '';
      sent.add(comment.id);
      list.append(node);
      $('comment-text').value = '';
      attachments.clear();
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

    attachments = initAttachments({
      link: $('comment-attach'),
      input: $('comment-file'),
      list: $('comment-photos'),
      error,
      onChange: () => {},
    });

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
    location.href = '/' + wallOf(post).nickname;
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
    viewer: me,
    onDelete: [post.author.id, wallOf(post).id].includes(me?.id) && removePost,
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
