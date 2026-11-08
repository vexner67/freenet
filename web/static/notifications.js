// "Уведомления", openvk's "Ответы" (Notification/Feed.latte): what others
// did with your posts and page, with the tabs "Непрочитанное" and "Архив".
// Each is a row of openvk's components/notifications: the photo of who did
// it, their name in bold, what they did with a bold link to it and the date
// in gray under it (a mention says the date in its sentence, as openvk's
// does). Opening the unread tab reads it: the notifications move to the
// archive and the menu counter clears; while it is open, new ones appear at
// the top live (events.js). Opened from the menu with nothing unread, the
// page shows the archive at once, as openvk's does. The guests of the page
// are rows here too: those who came since the last look above the unread
// notifications, all of the last 30 days above the archive. A subscriber
// of Prime with "Невидимка" leaves no trace.

import { api, currentUser, started } from './api.js';
import { onEvent } from './events.js';
import { callLength, conversationPath, formatPostDate, guestCame, notificationVerb, plainMentions, snippet, votesCount } from './labels.js';
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
    showEmpty();

    if (page.newestId) await markRead(page.newestId);
  } catch (e) {
    showError($('error'), e);
  }
}

// showEmpty is openvk's "Тут ничего нет... Пока..." when there is no row.
function showEmpty() {
  $('empty').hidden = $('notifications').children.length + $('guests-list').children.length > 0;
}

// bold is a link in bold, as openvk writes every one: <a><b>…</b></a>.
function bold(href, text) {
  return el('a', { href }, el('b', { textContent: text }));
}

// quote is what a comment or a post says, cut short, with the people it
// names in plain words.
function quote(text) {
  return snippet(plainMentions(text || tr('Фотография')), COMMENT_SNIPPET);
}

// said ends a sentence with what the message or the comment it is about
// says; with a full stop alone when it says nothing (photos only).
function said(text) {
  return text ? `: «${quote(text)}».` : '.';
}

// mention is openvk's sentence of a mention: "Вас упомянул пользователь
// Анна сегодня в 12:00 в посте или в одной из веток его обсуждения:
// «…»", with the date in it and no line of the date under it.
function mention(n, name) {
  const where = n.type === 'topic_mention'
    ? bold(topicPath(n.topic), tr('в обсуждении'))
    : bold(postPath(n.post), tr('в посте или в одной из веток его обсуждения'));
  const text = n.type === 'post_mention' ? n.post.text : n.comment?.text;

  return [n.actor.club ? tr('Вас упомянуло сообщество ') : tr('Вас упомянул пользователь '),
    name, ` ${formatPostDate(n.createdAt)} `, where, ...(text ? [`: «${quote(text)}»`] : [])];
}

// votes is openvk's "передал вам 10 голосов. Сообщение: «…»."; a transfer
// told before the sum was kept says only "голоса".
function votes(n) {
  const sum = n.votes ? votesCount(n.votes) : tr('голоса');

  return [tr(' {0} вам {1}.', notificationVerb('votes', n.actor), sum), ...(n.note ? [tr(' Сообщение: «{0}».', n.note)] : [])];
}

// notificationText is the sentence of openvk; null for a type this page
// does not know yet.
function notificationText(n) {
  const name = el('span', {}, bold(pagePath(n.actor), fullName(n.actor)), verifiedMark(n.actor));
  // A community acts by itself, as openvk says: "Сообщество Кошки ...".
  const who = n.actor.club ? [tr('Сообщество '), name] : [name];
  const verb = notificationVerbs[n.type] && notificationVerb(n.type, n.actor);

  switch (n.type) {
    case 'like':
      return [...who, tr(' {0} вашу ', verb), bold(postPath(n.post), tr('запись')), tr(' от {0}.', formatPostDate(n.post.createdAt))];
    case 'comment':
      return [...who, tr(' {0} комментарий под вашей ', verb), bold(postPath(n.post), tr('записью')),
        tr(' от {0}: «{1}».', formatPostDate(n.post.createdAt), quote(n.comment.text))];
    case 'comment_reply':
      return [...who, tr(' {0} на ваш комментарий под ', verb), bold(postPath(n.post), tr('записью')), said(n.comment.text)];
    case 'comment_like':
      return [...who, tr(' {0} ваш ', verb), bold(`${postPath(n.post)}#comments`, tr('комментарий')),
        tr(' от {0}.', formatPostDate(n.comment.createdAt))];
    case 'repost':
      return [...who, tr(' {0} вашей ', verb), bold(postPath(n.post), tr('записью')), tr(' от {0}.', formatPostDate(n.post.createdAt))];
    case 'wall_post':
      return [...who, tr(' {0} на вашей стене ', verb), bold(postPath(n.post), tr('запись')), said(n.post.text)];
    case 'post_mention':
    case 'comment_mention':
    case 'topic_mention':
      return mention(n, name);
    case 'post_accepted':
      return [tr('Сообщество '), name, tr(' опубликовало вашу предложенную '), bold(postPath(n.post), tr('запись')), '.'];
    // The community you run, as openvk says it; the link opens the
    // suggestions, which only its admins see.
    case 'suggested_post':
      return [tr('В сообществе '), bold(`${pagePath(n.actor)}/suggested`, fullName(n.actor)), verifiedMark(n.actor), tr(' новые записи в предложке')];
    // Made an admin by its owner; openvk names who did it, we the community.
    case 'club_admin':
      return [tr('Вас назначили руководителем сообщества '), bold(pagePath(n.actor), fullName(n.actor)), verifiedMark(n.actor), '.'];
    // A message in the topic you started, openvk's "оставил комментарий
    // под вашей темой".
    case 'topic_message':
      return [...who, tr(' {0} комментарий под вашей темой ', notificationVerb('comment', n.actor)),
        bold(topicPath(n.topic), `«${n.topic.title}»`), said(n.comment?.text)];
    // A heart on your message of a topic, as on a comment.
    case 'topic_message_like':
      return [...who, tr(' {0} ваше сообщение в обсуждении ', notificationVerb('like', n.actor)),
        bold(topicPath(n.topic), `«${n.topic.title}»`), said(n.comment?.text)];
    // openvk's "отправил вам подарок.", without a link as there.
    case 'gift':
      return [...who, tr(' {0} вам подарок.', verb)];
    case 'votes':
      return [...who, ...votes(n)];
    default:
      return ourText(n, who);
  }
}

// The verbs notificationVerb knows by the type itself.
const notificationVerbs = { like: 1, comment: 1, comment_reply: 1, comment_like: 1, repost: 1, wall_post: 1, gift: 1 };

// ourText is a notification openvk does not have, said as its are.
function ourText(n, who) {
  switch (n.type) {
    // An administrator of the site gave your page the checkmark; openvk
    // tells nobody.
    case 'verified':
      return [...who, tr(' {0} вашу страницу: теперь рядом с вашим именем стоит галочка.', notificationVerb('verified', n.actor))];
    // The community you own got the checkmark; the actor is the community.
    case 'club_verified':
      return [...who, tr(' подтверждено: теперь рядом с его названием стоит галочка.')];
    // A like and a comment of your story, as VK tells them; the story
    // lives a day, and the notification goes with it.
    case 'story_like':
      return [...who, tr(' {0} вашу ', notificationVerb('like', n.actor)), el('b', { textContent: tr('историю') }), '.'];
    case 'story_comment':
      return [...who, tr(' {0} комментарий под вашей ', notificationVerb('comment', n.actor)), el('b', { textContent: tr('историей') }), '.'];
    // A follow is a friend request, a follow back its acceptance, as the
    // requests of openvk's "Друзья".
    case 'follow':
      return [...who, tr(' хочет добавить вас в '), bold('/friends?section=requests', tr('друзья')), '.'];
    case 'friend':
      return [...who, tr(' {0} вашу заявку в ', notificationVerb('friend', n.actor)), bold('/friends', tr('друзья')), '.'];
    // Signed up by your link of "Пригласить" and follows you now.
    case 'invite':
      return [...who, tr(' {0} по вашему ', notificationVerb('invite', n.actor)), bold('/invite', tr('приглашению')), '.'];
    // Prime given as a present: its term is on its page.
    case 'prime':
      return [...who, tr(' {0} вам ', notificationVerb('prime', n.actor)), bold('/prime', 'Prime'), '.'];
    // A like of "Знакомства" answered with a like; the matches are there.
    case 'match':
      return [...who, tr(' — взаимная симпатия в '), bold('/dating', tr('знакомствах')), '.'];
    // Calls are told here, not in the dialog, which keeps only its button;
    // the link opens the dialog, to call back.
    case 'call':
      return [...who, ' — ', bold(conversationPath(`user:${n.actor.nickname}`), tr('звонок')), tr(', {0}.', callLength(n.seconds))];
    case 'missed_call':
      return [...who, ' — ', bold(conversationPath(`user:${n.actor.nickname}`), tr('пропущенный звонок')), '.'];
    default:
      return null;
  }
}

// Undated are the rows openvk writes without the date under them: a
// mention has it in its sentence, the suggestions of a community none.
const undated = new Set(['post_mention', 'comment_mention', 'topic_mention', 'post_accepted', 'suggested_post']);

// row is a row of openvk's list: the photo, 50 by 50, and the words.
function row(who, words, date, className = 'post notification') {
  return el('div', { className },
    el('a', { className: 'post-avatar', href: pagePath(who) }, el('img', { src: avatarURL(who, 'small'), alt: '' })),
    el('div', { className: 'post-main notification-text' }, ...words, ...(date ? [el('div', { className: 'nobold', textContent: formatPostDate(date) })] : [])));
}

function notificationNode(n) {
  const text = notificationText(n);
  if (!text) return null;

  return row(n.actor, text, undated.has(n.type) ? '' : n.createdAt);
}

// guestNode is a guest told as a notification: "Анна заходила на вашу
// страницу.", the visit under it.
function guestNode(guest) {
  const name = el('span', {}, bold(pagePath(guest.user), fullName(guest.user)), verifiedMark(guest.user));

  return row(guest.user, [name, guestCame(guest)], guest.visitedAt, guest.new ? 'post notification guest-row-new' : 'post notification');
}

let guestsCursor = '';

// showGuests puts the guests of a page above the notifications: on the
// unread tab only the new ones, on the archive all of them, page by page.
function showGuests(page) {
  const shown = box === 'unread' && unreadTab ? page.guests.filter((g) => g.new) : page.guests;
  $('guests-list').append(...shown.map(guestNode));
  guestsCursor = unreadTab ? '' : page.nextCursor;
  $('guests-more').hidden = !guestsCursor;

  const guestRows = $('guests-list').children.length;
  $('guests').hidden = guestRows === 0;
  // The archive is all notifications at once: guests there go untitled.
  $('guests-title').hidden = !unreadTab;
  showEmpty();
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
  $('more').hidden = true;

  try {
    const page = await (first ?? api.notifications(box, cursor));
    first = null;

    $('notifications').append(...nodesOf(page.notifications));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    showEmpty();

    if (box === 'unread' && page.newestId) {
      await api.markNotificationsRead(page.newestId);
      setCounter('notifications', 0);
      // Everything shown and older is in the archive now: the next pages
      // come from there, from the same cursor.
      box = 'archive';
    }
  } catch (e) {
    $('more').hidden = !cursor;
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
