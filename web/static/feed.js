import { api, currentUser } from './api.js';
import { $, el, isMicroblog, renderChrome, showError, hideError, autosize } from './layout.js';
import { postNode, wallOwnerID } from './posts.js';
import { initAttachments } from './attach.js';
import { initDocuments } from './docs.js';
import { initPollDraft } from './poll.js';
import { initTimer, postponedNotice } from './postponed.js';
import { openFeedSettings, readFeedSettings } from './feedsettings.js';

// openvk's limit of a post; the counter shows near the end.
const MAX_LENGTH = 60000;
const COUNTER_FROM = MAX_LENGTH - 100;

// "Все новости" of openvk, /feed/all: what everyone wrote, not only those
// I follow.
const all = location.pathname.replace(/\/+$/, '') === '/feed/all';

// "Настройки ленты": how many posts a page holds and whether the posts
// on the walls of others are there are the browser's; the ignored sources
// are shown only while the address says so, as on openvk.
const SHOW_IGNORED = 'ignored';

let settings = readFeedSettings();
const showIgnored = new URLSearchParams(location.search).get(SHOW_IGNORED) === '1';

let cursor = '';

async function loadMore(me) {
  hideError($('error'));

  try {
    const page = await api.feed(cursor, isMicroblog(), all, { limit: settings.posts, alien: settings.alien, ignored: showIgnored });

    $('posts').append(...page.posts.map((post) => feedNode(post, me)));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    showEmpty();
  } catch (e) {
    showError($('error'), e);
  }
}

// showEmpty says why there are no news: nobody is followed yet, or the
// settings of the feed leave everything out.
function showEmpty() {
  const empty = $('posts').children.length === 0;
  const narrowed = !settings.alien;

  $('empty-settings').hidden = !(empty && narrowed);
  $(all ? 'empty-all' : 'empty').hidden = !(empty && !narrowed);
}

// applySettings reads the news again as the box of settings said.
function applySettings(me, chosen, withIgnored) {
  if (withIgnored !== showIgnored) {
    const url = new URL(location.href);

    if (withIgnored) {
      url.searchParams.set(SHOW_IGNORED, '1');
    } else {
      url.searchParams.delete(SHOW_IGNORED);
    }

    location.assign(url);
    return;
  }

  settings = chosen;
  reload(me);
}

function reload(me) {
  cursor = '';
  $('posts').replaceChildren();
  $('more').hidden = true;
  loadMore(me);
}

// feedNode is a post as on a wall: I edit and delete what I wrote, and
// delete, pin and put away what is on my wall and on the walls of the
// communities I run (post.moderated).
function feedNode(post, me) {
  const moderates = Boolean(post.moderated);

  return postNode(post, {
    viewer: me,
    moderates,
    onDelete: (post.mine || moderates || wallOwnerID(post) === me.id) && remove,
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

  const documents = initDocuments({
    link: $('post-doc'),
    list: $('post-documents'),
    onChange: () => {
      if (documents.count()) form.classList.add('expanded');
    },
  });

  let poll = null;

  poll = initPollDraft({
    link: $('post-poll'),
    marker: $('post-poll-marker'),
    onChange: () => {
      if (poll?.value()) form.classList.add('expanded');
    },
  });

  // "Таймер": the post waits for its time on my wall.
  const timer = initTimer({
    link: $('post-timer'),
    box: $('post-timer-box'),
    submit: $('post-submit'),
    onChange: (on) => {
      if (on) form.classList.add('expanded');
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

    if (timer.active() && poll.value()) {
      showError(error, 'К отложенной записи нельзя прикрепить опрос.');
      return;
    }

    if (timer.active() && documents.count()) {
      showError(error, 'К отложенной записи нельзя прикрепить документ.');
      return;
    }

    $('post-submit').disabled = true;

    try {
      if (timer.active()) {
        const p = await api.postpone(text.value, attachments.ids(), $('post-nsfw').checked, timer.value());
        $('posts').prepend(postponedNotice(p, me));
        timer.clear();
      } else {
        $('posts').prepend(feedNode(await api.createPost(me.nickname, text.value, attachments.ids(), $('post-nsfw').checked, poll.value(), documents.ids()), me));
      }
      showEmpty();
      text.value = '';
      $('post-nsfw').checked = false;
      autosize(text);
      text.blur();
      attachments.clear();
      documents.clear();
      poll.clear();
      form.classList.remove('expanded');
      updateCounter();
    } catch (e) {
      showError(error, e);
    } finally {
      $('post-submit').disabled = false;
    }
  });
  text.addEventListener('focus', () => form.classList.add('expanded'));
  // The form stays open while it holds text, photos or a poll.
  text.addEventListener('blur', () => {
    if (!text.value.trim() && attachments.count() === 0 && documents.count() === 0 && !poll.value() && !timer.active()) form.classList.remove('expanded');
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
  $('feed-settings').addEventListener('click', () => openFeedSettings({
    showIgnored,
    onApply: (chosen, withIgnored) => applySettings(me, chosen, withIgnored),
    onUnignored: () => reload(me),
  }));
  await loadMore(me);
}
