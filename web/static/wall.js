// The wall block of a profile page: posts, the form for the owner, paging.

import { api } from './api.js';
import { postsCount } from './labels.js';
import { $, el, showError, hideError } from './layout.js';
import { postNode as renderPost } from './posts.js';

const COUNTER_FROM = 900;
const MAX_LENGTH = 1000;

// initWall: canLike for signed-in viewers, who may like the posts.
export function initWall(owner, isOwner, canLike) {
  const posts = $('posts');
  const error = $('wall-error');

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

    const button = $('post-submit');
    button.disabled = true;

    try {
      const post = await api.createPost($('post-text').value);

      posts.prepend(postNode(post));
      $('post-text').value = '';
      $('post-text').blur();
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

    form.hidden = false;
    form.addEventListener('submit', submit);
    text.addEventListener('focus', () => form.classList.add('expanded'));
    text.addEventListener('blur', () => {
      if (!text.value.trim()) form.classList.remove('expanded');
    });
    text.addEventListener('input', updateCounter);
    // Ctrl+Enter (Cmd+Enter on a Mac) sends, as on the old sites.
    text.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) form.requestSubmit();
    });
  }

  $('wall-more').addEventListener('click', loadMore);

  return loadMore();
}
