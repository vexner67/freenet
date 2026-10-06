// The wall block of a profile page: posts, the form for the owner, paging.

import { api } from './api.js';
import { postsCount } from './labels.js';
import { $, el, showError, hideError } from './layout.js';
import { postNode as renderPost } from './posts.js';
import { initAttachments } from './attach.js';

const COUNTER_FROM = 900;
const MAX_LENGTH = 1000;

// initWall: canLike for signed-in viewers, who may like the posts.
export function initWall(owner, isOwner, canLike) {
  const posts = $('posts');
  const error = $('wall-error');
  let attachments;

  let total = 0;
  let cursor = '';

  function updateCount() {
    $('wall-count').textContent = total > 0 ? ' · ' + postsCount(total) : '';
    $('wall-empty').hidden = total > 0 || isOwner;
  }

  const postNode = (post) => renderPost(post, {
    onDelete: isOwner && remove,
    canLike,
    onError: (e) => showError(error, e),
  });

  async function remove(post, node) {
    hideError(error);

    try {
      await api.deletePost(post.id);
    } catch (e) {
      showError(error, e);
      return;
    }

    node.replaceWith(el('div', { className: 'post-deleted', textContent: 'Запись удалена.' }));
    total--;
    updateCount();
  }

  async function loadMore() {
    const more = $('wall-more');
    hideError(error);

    try {
      const page = await api.wall(owner.nickname, cursor);

      total = page.total;
      cursor = page.nextCursor;
      posts.append(...page.posts.map(postNode));
      more.hidden = !cursor;
      updateCount();
    } catch (e) {
      showError(error, e);
    }
  }

  function updateCounter() {
    const left = MAX_LENGTH - $('post-text').value.length;
    $('post-counter').textContent = left <= MAX_LENGTH - COUNTER_FROM ? `осталось ${left}` : '';
  }

  async function submit(event) {
    event.preventDefault();
    hideError(error);

    if (attachments.busy()) {
      showError(error, 'Дождитесь, пока загрузятся фотографии.');
      return;
    }

    const button = $('post-submit');
    button.disabled = true;

    try {
      const post = await api.createPost($('post-text').value, attachments.ids());

      posts.prepend(postNode(post));
      $('post-text').value = '';
      $('post-text').blur();
      attachments.clear();
      $('post-form').classList.remove('expanded');
      total++;
      updateCount();
      updateCounter();
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  }

  if (isOwner) {
    const form = $('post-form');
    const text = $('post-text');

    // The form stays open while it holds text or photos.
    const collapseIfEmpty = () => {
      if (!text.value.trim() && attachments.count() === 0 && document.activeElement !== text) {
        form.classList.remove('expanded');
      }
    };

    attachments = initAttachments({
      link: $('post-attach'),
      input: $('post-file'),
      list: $('post-photos'),
      error,
      onChange: () => {
        if (attachments?.count()) form.classList.add('expanded');
      },
    });

    form.hidden = false;
    form.addEventListener('submit', submit);
    text.addEventListener('focus', () => form.classList.add('expanded'));
    text.addEventListener('blur', collapseIfEmpty);
    text.addEventListener('input', updateCounter);
    // Ctrl+Enter (Cmd+Enter on a Mac) sends, as on the old sites.
    text.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) form.requestSubmit();
    });
  }

  $('wall-more').addEventListener('click', loadMore);

  return loadMore();
}
