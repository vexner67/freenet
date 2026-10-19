// The service worker of the site: it shows the push notifications that
// notification sends (notification/internal/webpush) while no page of the
// site is in front, and opens the page one is about when it is clicked.
// It keeps nothing for working offline. Served at /sw.js, so that it may
// look after every page; a classic script, not a module, as browsers do
// not all start modules as workers.
'use strict';

// What someone did, in the words of "Мои Ответы" (notifications.js):
// [he, she, unknown, a community].
const notificationVerbs = {
  like: ['оценил', 'оценила', 'оценил(а)', 'оценило'],
  comment: ['оставил', 'оставила', 'оставил(а)', 'оставило'],
  follow: ['подписался', 'подписалась', 'подписался(-ась)', 'подписалось'],
  wall_post: ['написал', 'написала', 'написал(а)', 'написало'],
  repost: ['поделился', 'поделилась', 'поделился(-лась)', 'поделилось'],
  comment_reply: ['ответил', 'ответила', 'ответил(а)', 'ответило'],
  mention: ['упомянул', 'упомянула', 'упомянул(а)', 'упомянуло'],
  gift: ['отправил', 'отправила', 'отправил(а)', 'отправило'],
  votes: ['передал', 'передала', 'передал(а)', 'передало'],
  invite: ['присоединился', 'присоединилась', 'присоединился(-ась)', 'присоединилось'],
  verified: ['подтвердил', 'подтвердила', 'подтвердил(а)', 'подтвердило'],
};

// Each kind of notification: the verb and what follows it. The payload
// says nothing of the post or the comment: a push service would read it.
const pushTexts = {
  like: ['like', 'вашу запись'],
  comment: ['comment', 'комментарий под вашей записью'],
  follow: ['follow', 'на вас'],
  wall_post: ['wall_post', 'на вашей стене'],
  repost: ['repost', 'вашей записью'],
  comment_reply: ['comment_reply', 'на ваш комментарий'],
  comment_like: ['like', 'ваш комментарий'],
  topic_message: ['comment', 'комментарий под вашей темой'],
  topic_message_like: ['like', 'ваше сообщение в обсуждении'],
  post_mention: ['mention', 'вас в записи'],
  comment_mention: ['mention', 'вас в комментарии'],
  topic_mention: ['mention', 'вас в обсуждении'],
  photo_mention: ['mention', 'вас в комментарии под фотографией'],
  photo_like: ['like', 'вашу фотографию'],
  photo_comment: ['comment', 'комментарий под вашей фотографией'],
  note_comment: ['comment', 'комментарий под вашей заметкой'],
  gift: ['gift', 'вам подарок'],
  votes: ['votes', 'вам голоса'],
  invite: ['invite', 'по вашему приглашению'],
  verified: ['verified', 'вашу страницу'],
};

// Those said of the community itself, which is the title.
const clubTexts = {
  post_accepted: 'Опубликовало вашу предложенную запись.',
  suggested_post: 'Новые записи в предложке.',
  club_admin: 'Вас назначили руководителем сообщества.',
  club_verified: 'Сообщество подтверждено: у его названия теперь галочка.',
};

function verbOf(type, actor) {
  const [male, female, unknown, neuter] = notificationVerbs[type];
  if (actor.club) return neuter;
  return { male, female }[actor.gender] ?? unknown;
}

function capitalized(text) {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function clubPath(actor) {
  return '/club/' + encodeURIComponent(actor.nickname || actor.id);
}

// notificationOf is what a push shows: the title, the text, where a click
// leads, and the tag that puts one in place of another of the same; null
// for what this worker does not know.
function notificationOf(p) {
  const actor = p.actor || {};
  const name = actor.name || (actor.club ? 'Сообщество' : 'Freenet');

  if (p.type === 'message') {
    return { title: name, body: 'Новое сообщение.', url: '/im/' + encodeURIComponent(actor.nickname), tag: 'im:' + actor.id, renotify: true };
  }

  if (p.type === 'chat') {
    const chat = p.chat || {};
    return { title: chat.title || 'Беседа', body: name + ': новое сообщение.', url: '/im/chat/' + encodeURIComponent(chat.id), tag: 'chat:' + chat.id, renotify: true };
  }

  if (p.type !== 'notification') return null;

  const tag = 'n:' + p.id;

  if (clubTexts[p.kind]) {
    const url = { suggested_post: clubPath(actor) + '/suggested', club_verified: clubPath(actor) }[p.kind] ?? '/notifications';
    return { title: name, body: clubTexts[p.kind], url, tag };
  }

  const text = pushTexts[p.kind];
  if (!text) return null;

  const url = { gift: '/gifts', votes: '/settings?act=finance' }[p.kind] ?? '/notifications';

  return { title: name, body: capitalized(verbOf(text[0], actor) + ' ' + text[1] + '.'), url, tag };
}

// Safari takes the subscription back from a site whose pushes show
// nothing; the others let a page in front take the news itself.
function mustShow() {
  return /^((?!chrome|android).)*safari/i.test(self.navigator.userAgent);
}

// The number on the icon of the site on the home screen: the pages say
// what is unread (push.js showUnread), and each push of something new
// adds one until a page says it again. A worker lives only while it
// works, so the number is kept in its cache.
const UNREAD = '/unread';

async function unread() {
  const kept = await (await caches.open('freenet')).match(UNREAD);
  return kept ? Number(await kept.text()) || 0 : 0;
}

async function keepUnread(n) {
  await (await caches.open('freenet')).put(UNREAD, new Response(String(n)));
}

async function countPush() {
  if (!('setAppBadge' in self.navigator)) return;

  const n = await unread() + 1;
  await keepUnread(n);
  await self.navigator.setAppBadge(n);
}

async function show(payload) {
  const shown = notificationOf(payload);
  if (!shown) return;

  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  const inFront = windows.some((w) => w.focused);
  if (inFront && !mustShow()) return;

  // Another message of a dialog already in the shade is not one more
  // unread dialog.
  const fresh = (await self.registration.getNotifications({ tag: shown.tag })).length === 0;

  const icon = payload.actor?.avatar || '/static/icon-192.png';
  await self.registration.showNotification(shown.title, {
    body: shown.body,
    icon,
    // Android draws it, white, in the status bar.
    badge: '/static/badge.png',
    tag: shown.tag,
    renotify: Boolean(shown.renotify),
    data: { url: shown.url },
  });

  // The page in front shows it live already.
  if (inFront) {
    for (const n of await self.registration.getNotifications({ tag: shown.tag })) n.close();
    return;
  }

  if (fresh) await countPush().catch(() => {});
}

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));

self.addEventListener('push', (event) => {
  let payload;
  try {
    payload = event.data?.json();
  } catch {
    return;
  }
  if (payload) event.waitUntil(show(payload));
});

async function open(url) {
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  const page = windows.find((w) => 'navigate' in w);

  if (page) {
    const opened = await page.navigate(url).catch(() => null);
    if (opened) return opened.focus();
  }

  return self.clients.openWindow(url);
}

self.addEventListener('message', (event) => {
  const n = event.data?.unread;
  if (Number.isInteger(n) && n >= 0) event.waitUntil(keepUnread(n));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  event.waitUntil(open(event.notification.data?.url || '/'));
});
