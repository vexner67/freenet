import { api, currentUser } from './api.js';
import { $, el, isMicroblog, renderChrome, showError, hideError } from './layout.js';
import { postNode, wallOwnerID } from './posts.js';

// "Все новости" of openvk, /feed/all: what everyone wrote, not only those
// I follow.
const all = location.pathname.replace(/\/+$/, '') === '/feed/all';

let cursor = '';

async function loadMore(me) {
  hideError($('error'));

  try {
    const page = await api.feed(cursor, isMicroblog(), all);

    // As on a wall: I edit and delete what I wrote, and delete, pin and
    // put away what is on my wall.
    $('posts').append(...page.posts.map((post) => postNode(post, {
      viewer: me,
      onDelete: (post.mine || wallOwnerID(post) === me.id) && remove,
      onArchived: archived,
      canLike: true,
      onError: (e) => showError($('error'), e),
      onWallOf: me,
    })));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $(all ? 'empty-all' : 'empty').hidden = $('posts').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
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

// archived: a post put away leaves the news, as it leaves its wall.
function archived(post, node) {
  if (post.archived) gone(node, 'Запись перенесена в архив.');
}

function gone(node, text) {
  node.replaceWith(el('div', { className: 'post-deleted', textContent: text }));
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  $(all ? 'tab-all' : 'tab-mine').classList.add('active');
  $('example-url').textContent = `${location.host}/никнейм`;
  $('more').addEventListener('click', () => loadMore(me));
  await loadMore(me);
}
