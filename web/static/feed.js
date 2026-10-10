import { api, currentUser } from './api.js';
import { $, isMicroblog, renderChrome, showError, hideError } from './layout.js';
import { postNode } from './posts.js';

let cursor = '';

async function loadMore(me) {
  hideError($('error'));

  try {
    const page = await api.feed(cursor, isMicroblog());

    $('posts').append(...page.posts.map((post) => postNode(post, {
      canLike: true,
      onError: (e) => showError($('error'), e),
      onWallOf: me,
    })));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('posts').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  $('example-url').textContent = `${location.host}/никнейм`;
  $('more').addEventListener('click', () => loadMore(me));
  await loadMore(me);
}
