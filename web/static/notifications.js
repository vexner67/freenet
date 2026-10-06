// "Мои Ответы", as on openvk: what others did with your posts and page,
// with the tabs "Непрочитанное" and "Архив". Opening the unread tab reads
// it: the notifications move to the archive and the menu counter clears.

import { api, currentUser } from './api.js';
import { formatPostDate, notificationVerb, snippet } from './labels.js';
import { $, avatarURL, el, fullName, profilePath, renderChrome, setCounter, showError, hideError } from './layout.js';
import { postPath } from './posts.js';

const COMMENT_SNIPPET = 200;

let box = new URLSearchParams(location.search).get('box') === 'archive' ? 'archive' : 'unread';
let cursor = '';

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
        ` от ${formatPostDate(n.post.createdAt)}: «${snippet(n.comment.text, COMMENT_SNIPPET)}».`];
    case 'follow':
      return [name, ` ${verb} на вас.`];
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

    $('notifications').append(...page.notifications.map(notificationNode).filter(Boolean));
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
}
