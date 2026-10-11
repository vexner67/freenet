// openvk's "Закладки" (/fave): what I like — posts, comments and photos of
// albums, newest like first — under "Понравившиеся записи", and at
// /fave/own "Написанные мной записи": what I wrote, on whichever wall.
// Both are mine alone; the settings lead here, as on openvk.

import { api, currentUser } from './api.js';
import { favesCount, postsCount } from './labels.js';
import { $, adult, el, renderChrome, showError, hideError } from './layout.js';
import { postNode, wallOwnerID } from './posts.js';
import { commentNode } from './comments.js';
import { photoPath } from './albums.js';

const own = location.pathname.replace(/\/+$/, '') === '/fave/own';
const SECTIONS = ['posts', 'comments', 'photos'];
const asked = new URLSearchParams(location.search).get('section');
const section = SECTIONS.includes(asked) ? asked : 'posts';

// openvk's "faves_<section>_empty_tip".
const EMPTY = {
  posts: 'Здесь будут отображаться понравившиеся Вам записи.',
  comments: 'Здесь будут отображаться понравившиеся Вам комментарии.',
  photos: 'Здесь будут отображаться понравившиеся Вам фотографии.',
};

let cursor = '';
let total = 0;

function showCount() {
  $('summary-count').textContent = own ? (total === 0 ? 'Ни одной записи' : postsCount(total)) : favesCount(total);
  $('summary').hidden = false;
  $('empty').hidden = total > 0;
}

function gone(node, text) {
  node.replaceWith(el('div', { className: 'post-deleted', textContent: text }));
  total--;
  showCount();
}

async function remove(post, node) {
  hideError($('error'));

  try {
    await api.deletePost(post.id);
  } catch (e) {
    showError($('error'), e);
    return;
  }

  gone(node, 'Запись удалена.');
}

// A post as in the news: I edit and delete what I wrote, and delete, pin
// and put away what is on my wall.
function shownPost(post, me) {
  return postNode(post, {
    viewer: me,
    onDelete: (post.mine || wallOwnerID(post) === me.id) && remove,
    // What I archived on my wall stays among what I wrote; a liked post
    // put away leaves the faves.
    onArchived: (p, node) => { if (p.archived && !own) gone(node, 'Запись перенесена в архив.'); },
    canLike: true,
    onError: (e) => showError($('error'), e),
    onWallOf: me,
  });
}

// A comment leads to its post, as openvk's "correctLink".
function shownComment(comment) {
  return commentNode(comment, {
    canLike: true,
    onError: (e) => showError($('error'), e),
    href: '/post/' + encodeURIComponent(comment.postId),
  });
}

function shownPhoto(photo) {
  return el('div', { className: 'album-photo' },
    el('a', { href: photoPath(photo) }, adult(el('img', { src: photo.small, alt: photo.description, loading: 'lazy' }), photo.nsfw)));
}

async function loadMore(me) {
  hideError($('error'));

  try {
    const page = await (own ? api.ownPosts(cursor) : api.faves(section, cursor));

    if (!cursor) {
      total = page.total;
      showCount();
    }

    if (page.photos) $('photos').append(...page.photos.map(shownPhoto));
    else if (page.comments) $('posts').append(...page.comments.map(shownComment));
    else $('posts').append(...page.posts.map((post) => shownPost(post, me)));

    $('photos').hidden = $('photos').children.length === 0;
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  const title = own ? 'Написанные мной записи' : 'Понравившиеся записи';
  document.title = `${title} | Freenet`;
  $('title').textContent = title;
  $('empty').textContent = own ? 'Вы ещё ничего не написали.' : EMPTY[section];
  if (!own) {
    $('tabs').hidden = false;
    $('tab-' + section).classList.add('active');
  }
  $('more').addEventListener('click', () => loadMore(me));
  await loadMore(me);
}
