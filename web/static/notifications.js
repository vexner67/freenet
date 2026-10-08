// "Мои Ответы", as on openvk: what others did with your posts and page,
// with the tabs "Непрочитанное" and "Архив". Opening the unread tab reads
// it: the notifications move to the archive and the menu counter clears.
// While it is open, new ones appear at the top live (events.js).

import { api, currentUser } from './api.js';
import { onEvent } from './events.js';
import { formatPostDate, notificationVerb, snippet } from './labels.js';
import { $, avatarURL, el, fullName, profilePath, renderChrome, setCounter, showError, hideError } from './layout.js';
import { postPath } from './posts.js';

const COMMENT_SNIPPET = 200;

let box = new URLSearchParams(location.search).get('box') === 'archive' ? 'archive' : 'unread';
const unreadTab = box === 'unread';
let cursor = '';
// IDs of the notifications shown, so a live update repeats none.
const shownIDs = new Set();

function nodesOf(notifications) {
  const fresh = notifications.filter((n) => !shownIDs.has(n.id));
  for (const n of fresh) shownIDs.add(n.id);
  return fresh.map(notificationNode).filter(Boolean);
}

// unseen is the newest notification shown while the tab was hidden: it is
// read once the tab is seen.
let unseen = '';

async function markRead(newestId) {
  if (document.hidden) {
    unseen = newestId;
    return;
  }

  unseen = '';
  await api.markNotificationsRead(newestId);
  setCounter('notifications', 0);
}

// showNew puts what arrived since at the top and reads it, as opening the
// tab would.
async function showNew() {
  try {
    const page = await api.notifications('unread', '');

    $('notifications').prepend(...nodesOf(page.notifications));
    $('empty').hidden = $('notifications').children.length > 0;

    if (page.newestId) await markRead(page.newestId);
  } catch (e) {
    showError($('error'), e);
  }
}

function postLink(post, text) {
  return el('a', { href: postPath(post) }, el('b', { textContent: text }));
}

// notificationText is the sentence of openvk; null for a type this page
// does not know yet.
function notificationText(n) {
  const name = el('a', { className: 'post-name', href: '/' + n.actor.nickname, textContent: fullName(n.actor) });
  const verb = notificationVerb(n.type, n.actor.gender);

  switch (n.type) {
    case 'like':
      return [name, ` ${verb} вашу `, postLink(n.post, 'запись'), ` от ${formatPostDate(n.post.createdAt)}.`];
    case 'comment':
      return [name, ` ${verb} комментарий под вашей `, postLink(n.post, 'записью'),
        ` от ${formatPostDate(n.post.createdAt)}: «${snippet(n.comment.text || 'Фотография', COMMENT_SNIPPET)}».`];
    case 'comment_reply':
      return [name, ` ${verb} на ваш комментарий под `, postLink(n.post, 'записью'),
        ` от ${formatPostDate(n.post.createdAt)}: «${snippet(n.comment.text || 'Фотография', COMMENT_SNIPPET)}».`];
    case 'comment_like':
      return [name, ` ${verb} ваш комментарий под `, postLink(n.post, 'записью'),
        ` от ${formatPostDate(n.post.createdAt)}: «${snippet(n.comment.text || 'Фотография', COMMENT_SNIPPET)}».`];
    case 'follow':
      return [name, ` ${verb} на вас.`];
    case 'repost':
      return [name, ` ${verb} вашей `, postLink(n.post, 'записью'),
        ...(n.post.text ? [`: «${snippet(n.post.text, COMMENT_SNIPPET)}».`] : ['.'])];
    case 'wall_post':
      return [name, ` ${verb} на вашей стене `, postLink(n.post, 'запись'),
        ...(n.post.text ? [`: «${snippet(n.post.text, COMMENT_SNIPPET)}».`] : ['.'])];
    default:
      return null;
  }
}

function notificationNode(n) {
  const text = notificationText(n);
  if (!text) return null;

  return el('div', { className: 'post notification' },
    el('a', { className: 'post-avatar', href: '/' + n.actor.nickname }, el('img', { src: avatarURL(n.actor, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'notification-text' }, ...text),
      el('div', { className: 'post-date', textContent: formatPostDate(n.createdAt) }),
    ),
  );
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.notifications(box, cursor);

    $('notifications').append(...nodesOf(page.notifications));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('notifications').children.length > 0;

    if (box === 'unread' && page.newestId) {
      await api.markNotificationsRead(page.newestId);
      setCounter('notifications', 0);
      // Everything shown and older is in the archive now: the next pages
      // come from there, from the same cursor.
      box = 'archive';
    }
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  $('title').replaceChildren(
    el('a', { href: profilePath(me), textContent: fullName(me) || 'Моя страница' }),
    ' » Ответы',
  );
  $(box === 'unread' ? 'tab-unread' : 'tab-archive').classList.add('active');
  $('more').addEventListener('click', loadMore);

  await loadMore();

  if (unreadTab) {
    onEvent('notification', (e) => {
      if (e.change === 'new') showNew();
    });
    document.addEventListener('visibilitychange', () => {
      if (unseen) markRead(unseen).catch((e) => showError($('error'), e));
    });
  }
}
