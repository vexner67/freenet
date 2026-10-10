import { api, currentUser } from './api.js';
import { $, el, isMicroblog, renderChrome, showError, hideError, autosize } from './layout.js';
import { postNode, wallOwnerID } from './posts.js';
import { initAttachments } from './attach.js';

// openvk's limit of a post; the counter shows near the end.
const MAX_LENGTH = 60000;
const COUNTER_FROM = MAX_LENGTH - 100;

// "Все новости" of openvk, /feed/all: what everyone wrote, not only those
// I follow.
const all = location.pathname.replace(/\/+$/, '') === '/feed/all';

let cursor = '';

async function loadMore(me) {
  hideError($('error'));

  try {
    const page = await api.feed(cursor, isMicroblog(), all);

    $('posts').append(...page.posts.map((post) => feedNode(post, me)));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $(all ? 'empty-all' : 'empty').hidden = $('posts').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

// feedNode is a post as on a wall: I edit and delete what I wrote, and
// delete, pin and put away what is on my wall.
function feedNode(post, me) {
  return postNode(post, {
    viewer: me,
    onDelete: (post.mine || wallOwnerID(post) === me.id) && remove,
    onArchived: archived,
    canLike: true,
    onError: (e) => showError($('error'), e),
    onWallOf: me,
  });
}

// initForm is the form above the news, as on openvk: what I write there
// goes on my wall and shows first.
function initForm(me) {
  const form = $('post-form');
  const text = $('post-text');
  const error = $('error');

  // Declared first: the attachments call onChange while they are made.
  let attachments = null;

  attachments = initAttachments({
    link: $('post-attach'),
    input: $('post-file'),
    list: $('post-photos'),
    error,
    onChange: () => {
      if (attachments?.count()) form.classList.add('expanded');
    },
  });

  function updateCounter() {
    const left = MAX_LENGTH - text.value.length;
    $('post-counter').textContent = left <= MAX_LENGTH - COUNTER_FROM ? `осталось ${left}` : '';
  }

  form.hidden = false;
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);

    if (attachments.busy()) {
      showError(error, 'Дождитесь, пока загрузятся фотографии.');
      return;
    }

    $('post-submit').disabled = true;

    try {
      const post = await api.createPost(me.nickname, text.value, attachments.ids(), $('post-nsfw').checked);

      $('posts').prepend(feedNode(post, me));
      $('empty').hidden = true;
      $('empty-all').hidden = true;
      text.value = '';
      $('post-nsfw').checked = false;
      autosize(text);
      text.blur();
      attachments.clear();
      form.classList.remove('expanded');
      updateCounter();
    } catch (e) {
      showError(error, e);
    } finally {
      $('post-submit').disabled = false;
    }
  });
  text.addEventListener('focus', () => form.classList.add('expanded'));
  // The form stays open while it holds text or photos.
  text.addEventListener('blur', () => {
    if (!text.value.trim() && attachments.count() === 0) form.classList.remove('expanded');
  });
  text.addEventListener('input', updateCounter);
  // Ctrl+Enter (Cmd+Enter on a Mac) sends, as on a wall.
  text.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) form.requestSubmit();
  });
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
  // Without a nickname of your own you have no name to sign a post.
  if (me.profileCompleted) initForm(me);
  $('more').addEventListener('click', () => loadMore(me));
  await loadMore(me);
}
