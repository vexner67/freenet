// The wall block of a profile or community page: posts by the owner and
// by others, the form for whoever may write there, paging.

import { api } from './api.js';
import { postsCount } from './labels.js';
import { $, clubRef, el, showError, hideError } from './layout.js';
import { postNode as renderPost } from './posts.js';
import { initAttachments } from './attach.js';

const COUNTER_FROM = 900;
const MAX_LENGTH = 1000;

// initWall shows the wall of owner, a user or a community (owner.club), to
// me, the viewer (null if anonymous). Whoever moderates the wall, its owner
// or a community's admins, deletes and pins any post on it, others their
// own ones; who may write here the first page tells, as the owner's privacy
// or the community allows. An admin also speaks for the community, a box
// on the form ("От имени сообщества").
export function initWall(owner, me, { moderates = me?.id === owner.id, speaksForClub = false } = {}) {
  const posts = $('posts');
  const error = $('wall-error');
  const isOwner = me?.id === owner.id;
  const load = owner.club ? (c) => api.clubWall(clubRef(owner), c) : (c) => api.wall(owner.nickname, c);
  const create = owner.club
    ? (text, photos) => api.createClubPost(clubRef(owner), text, photos, $('post-from-club').checked)
    : (text, photos) => api.createPost(owner.nickname, text, photos);
  let attachments;
  let canPost = false;

  let total = 0;
  let cursor = '';

  function updateCount() {
    $('wall-count').textContent = total > 0 ? ' · ' + postsCount(total) : '';
    $('wall-empty').hidden = total > 0 || canPost;
  }

  const postNode = (post) => renderPost(post, {
    viewer: me,
    moderates,
    onDelete: (moderates || post.mine) && remove,
    // The pinned post comes first: the wall is read again from the top.
    onPinned: reload,
    canLike: Boolean(me),
    onError: (e) => showError(error, e),
  });

  function reload() {
    cursor = '';
    posts.replaceChildren();
    return loadMore();
  }

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
      const firstPage = !cursor;
      const page = await load(cursor);

      // Without a nickname of your own you have no name to sign a post.
      if (firstPage && page.canPost && me?.profileCompleted) showForm();

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
      const post = await create($('post-text').value, attachments.ids());

      // Below the pinned post, which stays first.
      const pinned = posts.firstElementChild?.classList.contains('pinned') ? posts.firstElementChild : null;
      if (pinned) pinned.after(postNode(post));
      else posts.prepend(postNode(post));
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

  function showForm() {
    const form = $('post-form');
    const text = $('post-text');

    canPost = true;
    // As on openvk: "Написать" on someone else's wall.
    if (!isOwner && !owner.club) text.placeholder = 'Написать';
    if (speaksForClub) $('post-from-club-label').hidden = false;

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
