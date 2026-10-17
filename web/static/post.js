// The page of a post at /post/<id>: the post, its comments oldest first and
// the form for a new comment, as on openvk.

import { api, currentUser, started } from './api.js';
import { initAttachments } from './attach.js';
import { initGraffiti } from './graffiti.js';
import { commentNode } from './comments.js';
import { genitiveName, replyMention } from './labels.js';
import { $, clubRef, el, fullName, pagePath, renderChrome, showError, hideError, autosize } from './layout.js';
import { postNode } from './posts.js';

// openvk's limit of a post or a comment; the counter shows near the end.
const MAX_LENGTH = 60000;
const COUNTER_FROM = MAX_LENGTH - 100;

const postID = decodeURIComponent(location.pathname.slice('/post/'.length));

// wallOf is the owner of the wall a post is on.
const wallOf = (post) => post.wall ?? post.author;

// renderTitle is "Владелец » Стена » Запись", as openvk heads a post with
// the owner of its wall.
function renderTitle(post) {
  const owner = wallOf(post);
  const path = pagePath(owner);
  const name = fullName(owner);

  document.title = owner.club ? `Запись в группе ${owner.name} | Freenet` : `Запись на стене ${genitiveName(owner)} | Freenet`;
  $('title').replaceChildren(
    el('a', { href: path, textContent: name }),
    ' » ',
    el('a', { href: path, textContent: 'Стена' }),
    ' » Запись',
  );
}

// moderatesWall tells whether the viewer keeps the wall of the post clean:
// its owner, or an admin of the community whose wall it is, who also
// comments for it ("От имени сообщества").
async function moderatesWall(post, me) {
  const owner = wallOf(post);
  if (!me) return false;
  if (!owner.club) return owner.id === me.id;

  const club = await api.club(clubRef(owner)).catch(() => null);

  return club?.role === 'admin' || club?.role === 'owner';
}

function initComments(post, me, moderates) {
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

  // The author of a comment, the author of the post and whoever keeps the
  // wall clean may delete it; mine tells the author a comment for the
  // community.
  const canDelete = (comment) => Boolean(me) && (moderates || post.mine || comment.mine || comment.author.id === me.id);
  // The admins of a community comment for it, if they tick the box.
  const speaksForClub = moderates && Boolean(wallOf(post).club);
  const commentNodeFor = (comment) => commentNode(comment, {
    onDelete: canDelete(comment) && remove,
    onReply: me?.profileCompleted && reply,
    canLike: Boolean(me),
    onError: (e) => showError(error, e),
  });

  // The comment the form answers, null for none.
  let replyTo = null;

  // reply starts an answer as openvk's "Ответить": the name of the author
  // first, in place of the name of an earlier answer.
  function reply(comment) {
    const text = $('comment-text');
    const mention = replyTo ? replyMention(replyTo.author) : null;
    const rest = mention && text.value.startsWith(mention) ? text.value.slice(mention.length) : text.value;

    replyTo = comment;
    text.value = replyMention(comment.author) + rest;
    autosize(text);
    $('comment-reply-to').hidden = false;
    text.focus();
    text.setSelectionRange(text.value.length, text.value.length);
  }

  function cancelReply() {
    replyTo = null;
    $('comment-reply-to').hidden = true;
  }

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
      const comment = await api.createComment(post.id, $('comment-text').value, attachments.ids(), {
        replyTo: replyTo?.id,
        fromClub: speaksForClub && $('comment-from-club').checked,
      });
      const node = commentNodeFor(comment);

      node.dataset.sent = '';
      sent.add(comment.id);
      list.append(node);
      $('comment-text').value = '';
      autosize($('comment-text'));
      attachments.clear();
      cancelReply();
      total++;
      updateCount();
      updateCounter();
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  }

  // closed shows that the owner of the wall lets nobody comment the post
  // anymore, in place of the form.
  function closed(isClosed) {
    $('comments-closed').hidden = !isClosed;
    if (me?.profileCompleted) $('comment-form').hidden = isClosed;
    else $('comment-hint').hidden = isClosed;
  }

  $('post').addEventListener('commentsclosed', (event) => closed(event.detail), true);

  if (me?.profileCompleted) {
    const form = $('comment-form');
    const text = $('comment-text');

    attachments = initAttachments({
      link: $('comment-attach'),
      also: [$('comment-graffiti')],
      input: $('comment-file'),
      list: $('comment-photos'),
      error,
      onChange: () => {},
    });
    initGraffiti({ link: $('comment-graffiti'), attachments });

    $('comment-from-club-label').hidden = !speaksForClub;
    form.hidden = false;
    form.addEventListener('submit', submit);
    $('comment-reply-cancel').addEventListener('click', cancelReply);
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

  closed(Boolean(post.commentsClosed));
  $('comments-more').addEventListener('click', loadMore);

  return loadMore();
}

async function removePost(post) {
  hideError($('error'));

  try {
    await api.deletePost(post.id);
    location.href = pagePath(wallOf(post));
  } catch (e) {
    showError($('error'), e);
  }
}

// The page is asked for with who is signed in, in one round trip.
const wanted = started(api.post(postID));
const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const post = await wanted;

  renderTitle(post);
  const moderates = await moderatesWall(post, me);
  $('post').append(postNode(post, {
    viewer: me,
    moderates,
    onDelete: (moderates || post.mine) && removePost,
    // The post stays on its page, which says where it is now.
    onArchived: () => {},
    canLike: Boolean(me),
    onError: (e) => showError($('error'), e),
    withCommentsLink: false,
  }));
  $('content').hidden = false;

  await initComments(post, me, moderates);
} catch (e) {
  const notFound = e.status === 404 || e.status === 400;
  $('title').textContent = notFound ? 'Запись не найдена' : 'Ошибка';
  document.title = $('title').textContent + ' | Freenet';
  showError($('error'), notFound ? 'Запись удалена или никогда не существовала.' : e);
}
