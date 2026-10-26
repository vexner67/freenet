// "Мои Ответы", as on openvk: what others did with your posts and page,
// with the tabs "Непрочитанное" and "Архив". Opening the unread tab reads
// it: the notifications move to the archive and the menu counter clears.
// While it is open, new ones appear at the top live (events.js). Opened
// from the menu with nothing unread, the page shows the archive at once.
// The guests of the page are here too: those who came since the last look
// above the unread notifications, all of the last 30 days above the
// archive. A subscriber of Freenet Prime with "Невидимка" leaves no trace.

import { photoPath } from './albums.js';
import { api, currentUser, started } from './api.js';
import { onEvent } from './events.js';
import { formatPostDate, guestCame, notificationVerb, plainMentions, snippet } from './labels.js';
import { $, avatarURL, el, fullName, guestsSeen, pagePath, profilePath, renderChrome, setCounter, showError, hideError, verifiedMark } from './layout.js';
import { postPath } from './posts.js';
import { topicPath } from './topics.js';
import { tr } from './i18n.js';

const COMMENT_SNIPPET = 200;

const asked = new URLSearchParams(location.search).get('box');
let box = asked === 'archive' ? 'archive' : 'unread';
let unreadTab = box === 'unread';
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

function photoLink(photo, text) {
  return el('a', { href: photoPath(photo) }, el('b', { textContent: text }));
}

// quote is what a comment or a post says, cut short, with the people it
// names in plain words.
function quote(text) {
  return snippet(plainMentions(text || tr('Фотография')), COMMENT_SNIPPET);
}

// said ends a sentence with what the message or the comment it is about
// says; with a full stop alone when it says nothing (photos only).
function said(n) {
  return n.comment?.text ? `: «${quote(n.comment.text)}».` : '.';
}

// notificationText is the sentence of openvk; null for a type this page
// does not know yet.
function notificationText(n) {
  const link = el('a', { className: 'post-name', href: pagePath(n.actor), textContent: fullName(n.actor) });
  // A community acts by itself, as openvk says: "Сообщество Кошки ...".
  const name = el('span', {}, ...(n.actor.club ? [tr('Сообщество ')] : []), link, verifiedMark(n.actor));

  if (n.type === 'post_accepted') {
    return [name, tr(' опубликовало вашу предложенную '), postLink(n.post, tr('запись')), '.'];
  }

  // The community you run, as openvk says it; the link opens the
  // suggestions, which only its admins see.
  if (n.type === 'suggested_post') {
    return [tr('В сообществе '), el('a', { href: `${pagePath(n.actor)}/suggested` }, el('b', { textContent: fullName(n.actor) })),
      verifiedMark(n.actor), tr(' новые записи в предложке')];
  }

  // Made an admin by its owner; openvk names who did it, we the community.
  if (n.type === 'club_admin') {
    return [tr('Вас назначили руководителем сообщества '), el('a', { href: pagePath(n.actor) }, el('b', { textContent: fullName(n.actor) })),
      verifiedMark(n.actor), '.'];
  }

  // An administrator of the site gave your page the checkmark; openvk
  // tells nobody.
  if (n.type === 'verified') {
    return [name, tr(' {0} вашу страницу: теперь рядом с вашим именем стоит галочка.', notificationVerb('verified', n.actor))];
  }

  // The community you own got the checkmark; the actor is the community.
  if (n.type === 'club_verified') {
    return [name, tr(' подтверждено: теперь рядом с его названием стоит галочка.')];
  }

  // A message in the topic you started, openvk's "оставил комментарий
  // под вашей темой".
  if (n.type === 'topic_message') {
    return [name, tr(' {0} комментарий под вашей темой ', notificationVerb('comment', n.actor)),
      el('a', { href: topicPath(n.topic) }, el('b', { textContent: `«${n.topic.title}»` })), said(n)];
  }

  // Named in a message of a topic: openvk's "Вас упомянул пользователь ...
  // в обсуждении", said as our other answers are.
  if (n.type === 'topic_mention') {
    return [name, tr(' {0} вас в обсуждении ', notificationVerb('mention', n.actor)),
      el('a', { href: topicPath(n.topic) }, el('b', { textContent: `«${n.topic.title}»` })), said(n)];
  }

  if (n.type === 'post_mention') {
    return [name, tr(' {0} вас в ', notificationVerb('mention', n.actor)), postLink(n.post, tr('записи')),
      ...(n.post.text ? [`: «${quote(n.post.text)}».`] : ['.'])];
  }

  if (n.type === 'comment_mention') {
    return [name, tr(' {0} вас в комментарии под ', notificationVerb('mention', n.actor)), postLink(n.post, tr('записью')),
      tr(' от {0}: «{1}».', formatPostDate(n.post.createdAt), quote(n.comment.text))];
  }

  // A heart on your message of a topic, as on a comment.
  if (n.type === 'topic_message_like') {
    return [name, tr(' {0} ваше сообщение в обсуждении ', notificationVerb('like', n.actor)),
      el('a', { href: topicPath(n.topic) }, el('b', { textContent: `«${n.topic.title}»` })), said(n)];
  }

  // A heart and a comment on a photo of your album; openvk's "оставил
  // комментарий под вашей фотографией".
  if (n.type === 'photo_like') {
    return [name, tr(' {0} вашу ', notificationVerb('like', n.actor)), photoLink(n.photo, tr('фотографию')), '.'];
  }

  // Named in a comment under a photo, whoever uploaded it.
  if (n.type === 'photo_mention') {
    return [name, tr(' {0} вас в комментарии под ', notificationVerb('mention', n.actor)), photoLink(n.photo, tr('фотографией')), said(n)];
  }

  if (n.type === 'photo_comment') {
    return [name, tr(' {0} комментарий под вашей ', notificationVerb('comment', n.actor)), photoLink(n.photo, tr('фотографией')), said(n)];
  }

  // A like and a comment of your story, as VK tells them; the story lives
  // a day, and the notification goes with it.
  if (n.type === 'story_like') {
    return [name, tr(' {0} вашу ', notificationVerb('like', n.actor)), el('b', { textContent: tr('историю') }), '.'];
  }

  if (n.type === 'story_comment') {
    return [name, tr(' {0} комментарий под вашей ', notificationVerb('comment', n.actor)), el('b', { textContent: tr('историей') }), '.'];
  }

  // openvk's "отправил вам подарок"; the gifts are on your page.
  if (n.type === 'gift') {
    return [name, tr(' {0} вам ', notificationVerb('gift', n.actor)), el('a', { href: '/gifts' }, el('b', { textContent: tr('подарок') })), '.'];
  }

  // Signed up by your link of "Пригласить" and follows you now.
  if (n.type === 'invite') {
    return [name, tr(' {0} по вашему ', notificationVerb('invite', n.actor)), el('a', { href: '/invite' }, el('b', { textContent: tr('приглашению') })), '.'];
  }

  // Freenet Prime given as a present: its term is on its page.
  if (n.type === 'prime') {
    return [name, tr(' {0} вам ', notificationVerb('prime', n.actor)), el('a', { href: '/prime' }, el('b', { textContent: 'Freenet Prime' })), '.'];
  }

  // openvk's "передал вам голоса"; the balance is in the settings.
  if (n.type === 'votes') {
    return [name, tr(' {0} вам ', notificationVerb('votes', n.actor)), el('a', { href: '/votes' }, el('b', { textContent: tr('голоса') })), '.'];
  }

  const verb = notificationVerb(n.type, n.actor);

  switch (n.type) {
    case 'like':
      return [name, tr(' {0} вашу ', verb), postLink(n.post, tr('запись')), tr(' от {0}.', formatPostDate(n.post.createdAt))];
    case 'comment':
      return [name, tr(' {0} комментарий под вашей ', verb), postLink(n.post, tr('записью')),
        tr(' от {0}: «{1}».', formatPostDate(n.post.createdAt), quote(n.comment.text))];
    case 'comment_reply':
      return [name, tr(' {0} на ваш комментарий под ', verb), postLink(n.post, tr('записью')),
        tr(' от {0}: «{1}».', formatPostDate(n.post.createdAt), quote(n.comment.text))];
    case 'comment_like':
      return [name, tr(' {0} ваш комментарий под ', verb), postLink(n.post, tr('записью')),
        tr(' от {0}: «{1}».', formatPostDate(n.post.createdAt), quote(n.comment.text))];
    case 'follow':
      return [name, tr(' {0} на вас.', verb)];
    case 'repost':
      return [name, tr(' {0} вашей ', verb), postLink(n.post, tr('записью')),
        ...(n.post.text ? [`: «${quote(n.post.text)}».`] : ['.'])];
    case 'wall_post':
      return [name, tr(' {0} на вашей стене ', verb), postLink(n.post, tr('запись')),
        ...(n.post.text ? [`: «${quote(n.post.text)}».`] : ['.'])];
    default:
      return null;
  }
}

function notificationNode(n) {
  const text = notificationText(n);
  if (!text) return null;

  return el('div', { className: 'post notification' },
    el('a', { className: 'post-avatar', href: pagePath(n.actor) }, el('img', { src: avatarURL(n.actor, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'notification-text' }, ...text),
      el('div', { className: 'post-date', textContent: formatPostDate(n.createdAt) }),
    ),
  );
}

// guestNode is a guest told as a notification: "Анна заходила на вашу
// страницу.", the visit under it.
function guestNode(guest) {
  const name = el('span', {},
    el('a', { className: 'post-name', href: pagePath(guest.user), textContent: fullName(guest.user) }),
    verifiedMark(guest.user));

  return el('div', { className: guest.new ? 'post notification guest-row-new' : 'post notification' },
    el('a', { className: 'post-avatar', href: pagePath(guest.user) }, el('img', { src: avatarURL(guest.user, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'notification-text' }, name, guestCame(guest)),
      el('div', { className: 'post-date', textContent: formatPostDate(guest.visitedAt) }),
    ),
  );
}

let guestsCursor = '';

// showGuests puts the guests of a page above the notifications: on the
// unread tab only the new ones, on the archive all of them, page by page.
function showGuests(page) {
  const shown = box === 'unread' && unreadTab ? page.guests.filter((g) => g.new) : page.guests;
  $('guests-list').append(...shown.map(guestNode));
  guestsCursor = unreadTab ? '' : page.nextCursor;
  $('guests-more').hidden = !guestsCursor;

  const count = $('guests-list').children.length;
  $('guests').hidden = count === 0;
  $('guests-title').textContent = unreadTab ? tr('Новые гости') : tr('Гости за 30 дней');
  if (count > 0) $('empty').hidden = true;
}

async function moreGuests() {
  hideError($('error'));
  $('guests-more').hidden = true;
  try {
    showGuests(await api.guests(guestsCursor));
  } catch (e) {
    $('guests-more').hidden = !guestsCursor;
    showError($('error'), e);
  }
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await (first ?? api.notifications(box, cursor));
    first = null;

    $('notifications').append(...nodesOf(page.notifications));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('notifications').children.length + $('guests-list').children.length > 0;

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

// The page is asked for with who is signed in, in one round trip; from
// the menu (no tab named) the archive too, should nothing be unread.
let first = started(api.notifications(box, ''));
const archive = asked ? null : started(api.notifications('archive', ''));
// The first page of guests makes them seen: the menu counter loses them.
const guests = started(api.guests());

// toArchive turns the page to the archive when the menu opened it with
// nothing unread: the tab "Непрочитанное" itself stays empty.
async function toArchive() {
  const page = await first.catch(() => null);
  const visits = await guests.catch(() => null);
  if (!archive || page?.notifications.length !== 0 || visits?.guests.some((g) => g.new)) return;

  box = 'archive';
  unreadTab = false;
  first = archive;
  history.replaceState(null, '', '/notifications?box=archive');
}
const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  $('title').replaceChildren(
    el('a', { href: profilePath(me), textContent: fullName(me) || tr('Моя страница') }),
    tr(' » Уведомления'),
  );
  await toArchive();
  $(box === 'unread' ? 'tab-unread' : 'tab-archive').classList.add('active');
  $('more').addEventListener('click', loadMore);
  $('guests-more').addEventListener('click', moreGuests);

  try {
    showGuests(await guests);
    guestsSeen();
  } catch (e) {
    showError($('error'), e);
  }
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
