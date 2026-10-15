// Client of the gateway API (same origin). Tokens live in localStorage;
// the strict Content-Security-Policy of the pages keeps foreign scripts out.

const STORAGE_KEY = 'freenet.tokens';

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export const session = {
  get() {
    try {
      return JSON.parse(localStorage.getItem(STORAGE_KEY));
    } catch {
      return null;
    }
  },
  save(tokens) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      accessToken: tokens.accessToken,
      refreshToken: tokens.refreshToken,
    }));
  },
  clear() {
    localStorage.removeItem(STORAGE_KEY);
  },
};

// request sends body as JSON, or as is when it is a file (a photo).
async function request(method, path, body, accessToken) {
  const isFile = body instanceof Blob;
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = isFile ? body.type || 'application/octet-stream' : 'application/json';
  if (accessToken) headers.Authorization = 'Bearer ' + accessToken;

  let response;
  try {
    response = await fetch(path, {
      method,
      headers,
      body: body === undefined || isFile ? body : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, 'network', 'network error');
  }

  if (response.status === 204) return null;

  let data = null;
  try {
    data = await response.json();
  } catch {
    // Not JSON, e.g. a proxy error page.
  }

  if (!response.ok) {
    throw new ApiError(response.status, data?.error?.code ?? 'internal', data?.error?.message ?? '');
  }

  return data;
}

// A refresh token works once, and reusing it ends the session. Tabs share
// localStorage, so they take a lock and skip refreshing if another tab
// already replaced the expired access token.
async function refresh(expiredAccessToken) {
  const run = async () => {
    const current = session.get();
    if (!current) throw new ApiError(401, 'unauthenticated', 'missing access token');
    if (current.accessToken !== expiredAccessToken) return;

    try {
      session.save(await request('POST', '/api/v1/auth/refresh', { refreshToken: current.refreshToken }));
    } catch (e) {
      if (e.status === 401) session.clear();
      throw e;
    }
  };

  await exclusive(run);
}

// exclusive runs what reads and replaces the tokens of the session while
// no other tab does: a refresh, or a switch to another account
// (accounts.js).
export async function exclusive(run) {
  if (typeof navigator !== 'undefined' && navigator.locks) return navigator.locks.request('freenet-refresh', run);

  return run();
}

// The requests of an account that is not the one signed in (accounts.js):
// new tokens for its refresh token, and a read with its access token.
export const renewTokens = (refreshToken) => request('POST', '/api/v1/auth/refresh', { refreshToken });
export const readAs = (accessToken, path) => request('GET', path, undefined, accessToken);

// renewSession replaces an access token that expired, for the event
// stream, which is no fetch; under the same lock as the requests.
export function renewSession(expiredAccessToken) {
  return refresh(expiredAccessToken);
}

async function withNewTokens(answer) {
  const me = await answer;
  await refresh(session.get()?.accessToken);
  return me;
}

// optional sends the token when signed in, so the answer can depend on who
// asks, and works anonymously otherwise.
function optional(method, path) {
  return session.get() ? authorized(method, path) : request(method, path);
}

async function authorized(method, path, body) {
  const current = session.get();
  if (!current) throw new ApiError(401, 'unauthenticated', 'missing access token');

  try {
    return await request(method, path, body, current.accessToken);
  } catch (e) {
    if (e.status !== 401) throw e;
  }

  await refresh(current.accessToken);

  return request(method, path, body, session.get().accessToken);
}

export const api = {
  requestCode: (email) => request('POST', '/api/v1/auth/code', { email }),

  async verifyCode(email, code) {
    session.save(await request('POST', '/api/v1/auth/verify', { email, code }));
  },

  async logout() {
    const current = session.get();
    session.clear();

    if (current) {
      try {
        await request('POST', '/api/v1/auth/logout', { refreshToken: current.refreshToken });
      } catch {
        // Signed out locally anyway; the session expires on its own.
      }
    }
  },

  me: () => authorized('GET', '/api/v1/me'),
  sessions: () => authorized('GET', '/api/v1/me/sessions'),
  endSession: (id) => authorized('DELETE', `/api/v1/me/sessions/${encodeURIComponent(id)}`),
  endOtherSessions: () => authorized('DELETE', '/api/v1/me/sessions'),
  updateProfile: (profile) => authorized('PUT', '/api/v1/me/profile', profile),
  // postLikers and commentLikers are openvk's "Понравилось N людям".
  postLikers: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/posts/${encodeURIComponent(id)}/likes`, cursor, limit)),
  // reposts are the posts sharing a post, newest first: who shared it.
  reposts: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/posts/${encodeURIComponent(id)}/reposts`, cursor, limit)),
  commentLikers: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/comments/${encodeURIComponent(id)}/likes`, cursor, limit)),
  // "Адрес страницы" of the settings: the nickname alone.
  updateNickname: (nickname) => authorized('PUT', '/api/v1/me/nickname', { nickname }),
  updateStatus: (status) => authorized('PUT', '/api/v1/me/status', { status }),
  updatePrivacy: (privacy) => authorized('PUT', '/api/v1/me/privacy', privacy),
  // "Интерфейс" of the settings: avatarStyle, mainPage and hiddenMenu.
  updateInterface: (ui) => authorized('PUT', '/api/v1/me/interface', ui),
  // "Контакты", "Интересы" and "Дополнительно" of "Редактировать страницу".
  updateContacts: (contacts) => authorized('PUT', '/api/v1/me/contacts', contacts),
  updateInterests: (interests) => authorized('PUT', '/api/v1/me/interests', interests),
  updateExtraFields: (fields) => authorized('PUT', '/api/v1/me/fields', { fields }),
  // Deleting and restoring the page change what its access tokens may do,
  // so the tokens are refreshed at once (under the lock, like any refresh).
  deactivate: (reason) => withNewTokens(authorized('POST', '/api/v1/me/deactivate', { reason })),
  reactivate: () => withNewTokens(authorized('POST', '/api/v1/me/reactivate')),
  // With a session the profile also says whether you follow the user.
  profile: (nickname) => optional('GET', '/api/v1/users/' + encodeURIComponent(nickname)),
  // As an anonymous visitor sees it: for the owner of a deleted page, whose
  // token is good only to restore it.
  publicProfile: (nickname) => request('GET', '/api/v1/users/' + encodeURIComponent(nickname)),

  // With a session posts also say whether you like them.
  // type is a tab of the wall: '' for all posts, 'owner', 'others' and,
  // for its owner, 'archive'; q is what the posts say ("Поиск").
  // comments: each post comes with its newest comments ("Микроблог").
  // year: only the posts of that year, as the years of the archive.
  wall: (nickname, cursor = '', type = '', q = '', comments = false, year = 0) => optional('GET',
    withType(page(`/api/v1/users/${encodeURIComponent(nickname)}/posts`, cursor, 0), type, q, comments, year)),
  // Puts every post of the wall written in year (0: in any) away in its
  // archive, or with archived = false brings them back.
  archiveWall: (nickname, year, archived = true) => authorized(archived ? 'PUT' : 'DELETE',
    withQuery(`/api/v1/users/${encodeURIComponent(nickname)}/posts/archive`, year ? 'year=' + year : '')),
  post: (id) => optional('GET', '/api/v1/posts/' + encodeURIComponent(id)),
  // Writes on the wall of nickname: someone else's or your own.
  // nsfw marks the post as adult ("Содержит NSFW-контент").
  // poll: the poll the post asks ({ title, options, anonymous, multiple,
  // locked, days }), null for none. documents are IDs of your documents.
  createPost: (nickname, text, photos = [], nsfw = false, poll = null, documents = []) =>
    authorized('POST', `/api/v1/users/${encodeURIComponent(nickname)}/posts`, { text, photos, nsfw, documents, ...(poll && { poll }) }),
  // VK's "Таймер": a post kept to come out at publishAt (ISO) on your own
  // wall, or, as postponeClubPost, speaking for a club you run.
  postpone: (text, photos, nsfw, publishAt) => authorized('POST', '/api/v1/me/postponed', { text, photos, nsfw, publishAt }),
  postponeClubPost: (ref, text, photos, { signed = false, nsfw = false, publishAt }) =>
    authorized('POST', `/api/v1/clubs/${encodeURIComponent(ref)}/postponed`, { text, photos, signed, nsfw, publishAt }),
  // The posts that wait for your wall or a club's, the soonest first.
  myPostponed: () => authorized('GET', '/api/v1/me/postponed'),
  clubPostponed: (ref) => authorized('GET', `/api/v1/clubs/${encodeURIComponent(ref)}/postponed`),
  postponedPost: (id) => authorized('GET', `/api/v1/postponed/${encodeURIComponent(id)}`),
  editPostponed: (id, text, nsfw, publishAt) => authorized('PATCH', `/api/v1/postponed/${encodeURIComponent(id)}`, { text, nsfw, publishAt }),
  // "Опубликовать сейчас": the post it became, null if it is not shown.
  publishPostponed: (id) => authorized('POST', `/api/v1/postponed/${encodeURIComponent(id)}/publish`),
  deletePostponed: (id) => authorized('DELETE', `/api/v1/postponed/${encodeURIComponent(id)}`),
  // Uploads a photo for a post; the post refers to it by ID.
  uploadPhoto: (file) => authorized('POST', '/api/v1/me/photos', file),
  // "Мои Документы": a file up to 10 MB, named by its file name; posts
  // and messages attach it by ID.
  documents(cursor = '') {
    return authorized('GET', withQuery('/api/v1/me/documents', cursor ? 'cursor=' + encodeURIComponent(cursor) : ''));
  },
  uploadDocument: (file) => authorized('POST', '/api/v1/me/documents?name=' + encodeURIComponent(file.name), file),
  renameDocument: (id, name) => authorized('PATCH', `/api/v1/documents/${encodeURIComponent(id)}`, { name }),
  deleteDocument: (id) => authorized('DELETE', `/api/v1/documents/${encodeURIComponent(id)}`),
  // Shares a post on your wall, with a comment that may be empty.
  // to is { club, fromClub, signed } to share it in a community ("в группу").
  // to: the community and how to sign, and the photos added to the words.
  repost: (postID, text, to = {}) => authorized('POST', `/api/v1/posts/${encodeURIComponent(postID)}/reposts`, { text, ...to }),
  deletePost: (id) => authorized('DELETE', '/api/v1/posts/' + encodeURIComponent(id)),
  editPost: (id, text, photos = [], nsfw = false) => authorized('PATCH', '/api/v1/posts/' + encodeURIComponent(id), { text, photos, nsfw }),
  pinPost: (id) => authorized('PUT', `/api/v1/posts/${encodeURIComponent(id)}/pin`),
  unpinPost: (id) => authorized('DELETE', `/api/v1/posts/${encodeURIComponent(id)}/pin`),
  // The owner of the wall puts a post away in "Архив записей" and back,
  // and closes and opens its comments.
  archivePost: (id) => authorized('PUT', `/api/v1/posts/${encodeURIComponent(id)}/archive`),
  unarchivePost: (id) => authorized('DELETE', `/api/v1/posts/${encodeURIComponent(id)}/archive`),
  closeComments: (id) => authorized('PUT', `/api/v1/posts/${encodeURIComponent(id)}/closed-comments`),
  openComments: (id) => authorized('DELETE', `/api/v1/posts/${encodeURIComponent(id)}/closed-comments`),
  // The poll of a post: options are the places of those chosen; both
  // answer with the poll as it became. pollVoters: who chose an option.
  votePoll: (postID, options) => authorized('PUT', `/api/v1/posts/${encodeURIComponent(postID)}/poll/vote`, { options }),
  retractVote: (postID) => authorized('DELETE', `/api/v1/posts/${encodeURIComponent(postID)}/poll/vote`),
  pollVoters(postID, option, cursor = '', limit = 0) {
    const path = page(`/api/v1/posts/${encodeURIComponent(postID)}/poll/voters`, cursor, limit);
    return optional('GET', path + (path.includes('?') ? '&' : '?') + 'option=' + option);
  },
  like: (postID) => authorized('PUT', `/api/v1/posts/${encodeURIComponent(postID)}/like`),
  unlike: (postID) => authorized('DELETE', `/api/v1/posts/${encodeURIComponent(postID)}/like`),

  // With a session comments also say whether you like them.
  // commentText is all a comment says: lists carry the beginning.
  commentText: (id) => optional('GET', `/api/v1/comments/${encodeURIComponent(id)}/text`),
  comments: (postID, cursor = '') => optional('GET', page(`/api/v1/posts/${encodeURIComponent(postID)}/comments`, cursor, 0)),
  // replyTo answers a comment of the post, "" for none; fromClub speaks for
  // the community of its wall ("От имени сообщества").
  createComment: (postID, text, photos = [], { replyTo = '', fromClub = false } = {}) =>
    authorized('POST', `/api/v1/posts/${encodeURIComponent(postID)}/comments`, { text, photos, fromClub, ...(replyTo && { replyTo }) }),
  likeComment: (id) => authorized('PUT', `/api/v1/comments/${encodeURIComponent(id)}/like`),
  unlikeComment: (id) => authorized('DELETE', `/api/v1/comments/${encodeURIComponent(id)}/like`),
  deleteComment: (id) => authorized('DELETE', '/api/v1/comments/' + encodeURIComponent(id)),

  follow: (nickname) => authorized('POST', `/api/v1/users/${encodeURIComponent(nickname)}/follow`),
  unfollow: (nickname) => authorized('DELETE', `/api/v1/users/${encodeURIComponent(nickname)}/follow`),
  // The blacklist ("Чёрный список"); blocking drops the follows both ways.
  block: (nickname) => authorized('PUT', `/api/v1/me/blacklist/${encodeURIComponent(nickname)}`),
  unblock: (nickname) => authorized('DELETE', `/api/v1/me/blacklist/${encodeURIComponent(nickname)}`),
  blacklist: (cursor = '') => authorized('GET', page('/api/v1/me/blacklist', cursor, 0)),
  followers: (nickname, cursor = '', limit = 0) => optional('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/followers`, cursor, limit)),
  following: (nickname, cursor = '', limit = 0) => optional('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/following`, cursor, limit)),
  // online is openvk's "Друзья онлайн": whom the user follows who is online now.
  online: (nickname, cursor = '', limit = 0) => {
    const path = page(`/api/v1/users/${encodeURIComponent(nickname)}/following`, cursor, limit);
    return optional('GET', path + (path.includes('?') ? '&' : '?') + 'online=true');
  },
  // common is openvk's "Общие друзья": whom the user and you both follow.
  common: (nickname, cursor = '', limit = 0) => {
    const path = page(`/api/v1/users/${encodeURIComponent(nickname)}/following`, cursor, limit);
    return authorized('GET', path + (path.includes('?') ? '&' : '?') + 'common=true');
  },
  // feed: your news, or with all those of everyone ("Все новости").
  // settings are "Настройки ленты": limit posts a page, alien (posts on
  // the walls of others) and ignored (the ignored sources too).
  feed: (cursor = '', comments = false, all = false, settings = {}) => authorized('GET', feedPath(cursor, comments, all, settings)),
  // "Игнорируемые источники": the walls the news leave out.
  ignored: () => authorized('GET', '/api/v1/me/feed/ignored'),
  unignore: (ids) => authorized('DELETE', '/api/v1/me/feed/ignored?' + ids.map((id) => 'id=' + encodeURIComponent(id)).join('&')),
  ignoreUser: (nickname) => authorized('PUT', `/api/v1/me/feed/ignored/users/${encodeURIComponent(nickname)}`),
  unignoreUser: (nickname) => authorized('DELETE', `/api/v1/me/feed/ignored/users/${encodeURIComponent(nickname)}`),
  ignoreClub: (ref) => authorized('PUT', `/api/v1/me/feed/ignored/clubs/${encodeURIComponent(ref)}`),
  unignoreClub: (ref) => authorized('DELETE', `/api/v1/me/feed/ignored/clubs/${encodeURIComponent(ref)}`),

  // onWall: post the photo on your wall too ("Опубликовать запись на стене").
  uploadAvatar: (file, onWall = false) => authorized('PUT', '/api/v1/me/avatar' + (onWall ? '?onWall=true' : ''), file),
  deleteAvatar: () => authorized('DELETE', '/api/v1/me/avatar'),
  // setBackdrop is a side, "left" or "right", of "Фон страницы".
  setBackdrop: (side, file) => authorized('PUT', `/api/v1/me/backdrop/${side}`, file),
  deleteBackdrop: () => authorized('DELETE', '/api/v1/me/backdrop'),

  notifications(box, cursor = '') {
    const query = new URLSearchParams({ box });
    if (cursor) query.set('cursor', cursor);
    return authorized('GET', '/api/v1/notifications?' + query);
  },
  unreadNotifications: () => authorized('GET', '/api/v1/notifications/unread-count'),
  markNotificationsRead: (upTo) => authorized('POST', '/api/v1/notifications/read', { upTo }),
  // Push notifications (push.js): the key of the server, and the
  // subscription of this browser.
  pushKey: () => request('GET', '/api/v1/push/key'),
  subscribePush: (subscription) => authorized('PUT', '/api/v1/me/push', subscription),
  unsubscribePush: (endpoint) => authorized('DELETE', '/api/v1/me/push', { endpoint }),

  dialogs: (cursor = '') => authorized('GET', page('/api/v1/dialogs', cursor, 0)),
  unreadDialogs: () => authorized('GET', '/api/v1/dialogs/unread-count'),
  // Older messages with cursor, newer ones with after (polling), or the
  // part of the dialog around a message, as a found one opens. What they
  // say is sealed: e2ee.js opens it.
  messages(nickname, { cursor = '', after = '', around = '', limit = 0 } = {}) {
    const query = new URLSearchParams();
    if (cursor) query.set('cursor', cursor);
    if (after) query.set('after', after);
    if (around) query.set('around', around);
    if (limit) query.set('limit', String(limit));
    const path = `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages`;
    return authorized('GET', query.size ? `${path}?${query}` : path);
  },
  // sealed is what the message says, sealed by e2ee.js (sendSealed):
  // plain words are refused. post shares a post into the dialog
  // ("Поделиться" "в переписку"), albumPhoto a photo of an album; forward
  // carries messages of your dialogs and chats on ("Переслать").
  sendMessage: (nickname, { sealed = null, photos = [], post = '', forward = [], albumPhoto = '', documents = [] } = {}) =>
    authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages`, { sealed, photos, post, forward, albumPhoto, documents }),
  // A sticker is a message by itself, not sealed: it says nothing.
  sendSticker: (nickname, sticker) => authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages`, { sticker }),
  // Chats of several people ("беседы"): members are nicknames.
  createChat: (title, members) => authorized('POST', '/api/v1/chats', { title, members }),
  chat: (id) => authorized('GET', `/api/v1/chats/${encodeURIComponent(id)}`),
  renameChat: (id, title) => authorized('PATCH', `/api/v1/chats/${encodeURIComponent(id)}`, { title }),
  // One message is pinned above a chat at a time; anyone in it pins.
  pinChatMessage: (id, messageId) => authorized('PUT', `/api/v1/chats/${encodeURIComponent(id)}/pin`, { messageId }),
  unpinChatMessage: (id) => authorized('DELETE', `/api/v1/chats/${encodeURIComponent(id)}/pin`),
  // The photo of a chat, a square cut out by avatar.js.
  uploadChatPhoto: (id, file) => authorized('PUT', `/api/v1/chats/${encodeURIComponent(id)}/photo`, file),
  deleteChatPhoto: (id) => authorized('DELETE', `/api/v1/chats/${encodeURIComponent(id)}/photo`),
  addChatMembers: (id, members) => authorized('POST', `/api/v1/chats/${encodeURIComponent(id)}/members`, { members }),
  removeChatMember: (id, nickname) => authorized('DELETE', `/api/v1/chats/${encodeURIComponent(id)}/members/${encodeURIComponent(nickname)}`),
  leaveChat: (id) => authorized('DELETE', `/api/v1/chats/${encodeURIComponent(id)}/membership`),
  returnToChat: (id) => authorized('PUT', `/api/v1/chats/${encodeURIComponent(id)}/membership`),
  // Older messages with cursor, newer ones with after (polling).
  chatMessages(id, { cursor = '', after = '', around = '' } = {}) {
    const query = new URLSearchParams();
    if (cursor) query.set('cursor', cursor);
    if (after) query.set('after', after);
    if (around) query.set('around', around);
    const path = `/api/v1/chats/${encodeURIComponent(id)}/messages`;
    return authorized('GET', query.size ? `${path}?${query}` : path);
  },
  // As a message of a dialog, it may share a post or a photo of an album
  // and carry messages on.
  sendChatMessage: (id, text, photos = [], post = '', forward = [], albumPhoto = '', documents = []) =>
    authorized('POST', `/api/v1/chats/${encodeURIComponent(id)}/messages`, { text, photos, post, forward, albumPhoto, documents }),
  sendChatSticker: (id, sticker) => authorized('POST', `/api/v1/chats/${encodeURIComponent(id)}/messages`, { sticker }),
  markChatRead: (id, upTo) => authorized('POST', `/api/v1/chats/${encodeURIComponent(id)}/read`, { upTo }),
  chatTyping: (id) => authorized('POST', `/api/v1/chats/${encodeURIComponent(id)}/typing`),
  // A message of a chat whole; your own is edited and taken back within
  // a day.
  chatMessage: (id) => authorized('GET', `/api/v1/chat-messages/${encodeURIComponent(id)}`),
  // base is the text the edit started from: the server refuses the edit if
  // the message says something else by now.
  editChatMessage: (id, text, { add = [], remove = [], base } = {}) =>
    authorized('PATCH', `/api/v1/chat-messages/${encodeURIComponent(id)}`, { text, baseText: base, addPhotos: add, removePhotos: remove }),
  deleteChatMessage: (id) => authorized('DELETE', `/api/v1/chat-messages/${encodeURIComponent(id)}`),
  markDialogRead: (nickname, upTo) => authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/read`, { upTo }),
  dialogTyping: (nickname) => authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/typing`),
  // Your own message, from both sides, within a day of sending: it stays
  // in the dialog as "Сообщение удалено"; forMe
  // deletes any message of your dialog for you alone ("Удалить у себя").
  deleteMessage: (id, { forMe = false } = {}) => authorized('DELETE', `/api/v1/messages/${encodeURIComponent(id)}` + (forMe ? '?for=me' : '')),
  // What your own message says, within a day of sending.
  // The edit names the photos it adds and takes away, not those left:
  // an edit from another window is not undone. sealed and base, what the
  // message said when the edit began ('' for nothing), as e2ee.js seals
  // them (editSealed).
  editMessage: (id, { sealed = null, base, add = [], remove = [] } = {}) =>
    authorized('PATCH', `/api/v1/messages/${encodeURIComponent(id)}`, { sealed, baseSealed: base, addPhotos: add, removePhotos: remove }),
  // The public keys messages of dialogs are sealed with (base64, null for
  // none): your own as the server has it, and another's by nickname.
  myMessageKey: () => authorized('GET', '/api/v1/me/message-key'),
  setMessageKey: (publicKey) => authorized('PUT', '/api/v1/me/message-key', { publicKey }),
  messageKey: (nickname) => authorized('GET', `/api/v1/users/${encodeURIComponent(nickname)}/message-key`),
  // Several messages at once, all or none (up to 100).
  deleteMessages(ids, { forMe = false } = {}) {
    const query = new URLSearchParams(ids.map((id) => ['id', id]));
    if (forMe) query.set('for', 'me');
    return authorized('DELETE', '/api/v1/messages?' + query);
  },
  // Your messages of chats that say q, newest first: with chat, an ID, in
  // that chat. The server cannot read dialogs: e2ee.js searches them.
  searchMessages(q, { chat = '', cursor = '' } = {}) {
    const query = new URLSearchParams({ q });
    if (chat) query.set('chat', chat);
    if (cursor) query.set('cursor', cursor);
    return authorized('GET', '/api/v1/messages/search?' + query);
  },

  // Communities ("группы"); ref is a club's address or, without one, its ID.
  createClub: (name, description) => authorized('POST', '/api/v1/clubs', { name, description }),
  // With a session the club also says what you are in it.
  club: (ref) => optional('GET', '/api/v1/clubs/' + encodeURIComponent(ref)),
  updateClub: (ref, settings) => authorized('PUT', '/api/v1/clubs/' + encodeURIComponent(ref), settings),
  // An event ("встреча"): startsAt and endsAt are ISO times, endsAt '' for none.
  // What the page "о сайте" counts: users, online, clubs, posts.
  about: () => request('GET', '/api/v1/about'),
  // Which of the nicknames (100 at most) are someone's: { nicknames }.
  knownNicknames: (nicknames) =>
    request('GET', '/api/v1/nicknames?' + nicknames.map((n) => 'nickname=' + encodeURIComponent(n)).join('&')),
  createEvent: (event) => authorized('POST', '/api/v1/events', event),
  // maybe: "Возможно приду" of an event.
  joinClub: (ref, { maybe = false } = {}) =>
    authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/membership${maybe ? '?maybe=true' : ''}`),
  leaveClub: (ref) => authorized('DELETE', `/api/v1/clubs/${encodeURIComponent(ref)}/membership`),
  // admins: only the owner and the admins ("Руководство"); going: 'sure'
  // or 'maybe', those who come to the event or only might.
  clubMembers(ref, { cursor = '', limit = 0, admins = false, going = '' } = {}) {
    const path = page(`/api/v1/clubs/${encodeURIComponent(ref)}/members`, cursor, limit);
    const filter = admins ? 'admins=true' : going && 'going=' + going;
    // With the token: the club tells those who run it whom it hides.
    return optional('GET', filter ? path + (path.includes('?') ? '&' : '?') + filter : path);
  },
  // Those who asked to join a closed group ("Заявки"), for those who run
  // it: { people, nextCursor, total }.
  clubRequests: (ref, { cursor = '', limit = 0 } = {}) =>
    authorized('GET', page(`/api/v1/clubs/${encodeURIComponent(ref)}/requests`, cursor, limit)),
  acceptClubRequest: (ref, nickname) => authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/requests/${encodeURIComponent(nickname)}`),
  rejectClubRequest: (ref, nickname) => authorized('DELETE', `/api/v1/clubs/${encodeURIComponent(ref)}/requests/${encodeURIComponent(nickname)}`),
  appointAdmin: (ref, nickname) => authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/admins/${encodeURIComponent(nickname)}`),
  dismissAdmin: (ref, nickname) => authorized('DELETE', `/api/v1/clubs/${encodeURIComponent(ref)}/admins/${encodeURIComponent(nickname)}`),
  // Those who run a club give one of them a title and hide or show them
  // ("Изменить комментарий", "Скрыт").
  // baseTitle is the title the change started from: the server refuses to
  // write over another.
  describeAdmin: (ref, nickname, { title, baseTitle, hidden }) =>
    authorized('PATCH', `/api/v1/clubs/${encodeURIComponent(ref)}/admins/${encodeURIComponent(nickname)}`, { title, baseTitle, hidden }),
  // The owner hands the club over to one of its admins ("Назначить владельцем").
  transferClub: (ref, nickname) => authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/owner/${encodeURIComponent(nickname)}`),
  // The clubs of a user, as their "Кому видно мои группы" allows: 403 if hidden.
  userClubs: (nickname, cursor = '', limit = 0) => optional('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/clubs`, cursor, limit)),
  // managed: only the clubs you run ("Управляемые").
  myClubs(cursor = '', managed = false) {
    const path = page('/api/v1/me/clubs', cursor, 0);
    return authorized('GET', managed ? path + (path.includes('?') ? '&' : '?') + 'managed=true' : path);
  },
  // "Мои Встречи": the same for the events you come to; time is
  // 'upcoming' for those ahead, 'past' for those over, '' for all.
  myEvents(cursor = '', managed = false, time = '') {
    const path = withQuery(page('/api/v1/me/events', cursor, 0), time && 'time=' + time);
    return authorized('GET', withQuery(path, managed && 'managed=true'));
  },
  // The block "Встречи" of a page and its list, shown as the groups are.
  userEvents: (nickname, time = '', cursor = '', limit = 0) => optional('GET',
    withQuery(page(`/api/v1/users/${encodeURIComponent(nickname)}/events`, cursor, limit), time && 'time=' + time)),
  // The clubs you keep in the left menu ("Добавить в левое меню").
  pinnedClubs: () => authorized('GET', '/api/v1/me/clubs/pinned'),
  pinClub: (ref) => authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/pin`),
  unpinClub: (ref) => authorized('DELETE', `/api/v1/clubs/${encodeURIComponent(ref)}/pin`),
  searchClubs(query, cursor = '') {
    const params = new URLSearchParams({ q: query });
    if (cursor) params.set('cursor', cursor);
    return request('GET', '/api/v1/clubs?' + params);
  },
  uploadClubAvatar: (ref, file, onWall = false) =>
    authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/avatar` + (onWall ? '?onWall=true' : ''), file),
  deleteClubAvatar: (ref) => authorized('DELETE', `/api/v1/clubs/${encodeURIComponent(ref)}/avatar`),
  // "Фон страницы": side is left or right; deleting takes both away.
  setClubBackdrop: (ref, side, file) => authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/backdrop/${side}`, file),
  deleteClubBackdrop: (ref) => authorized('DELETE', `/api/v1/clubs/${encodeURIComponent(ref)}/backdrop`),
  clubWall: (ref, cursor = '', type = '', q = '', comments = false, year = 0) => optional('GET',
    withType(page(`/api/v1/clubs/${encodeURIComponent(ref)}/posts`, cursor, 0), type, q, comments, year)),
  archiveClubWall: (ref, year, archived = true) => authorized(archived ? 'PUT' : 'DELETE',
    withQuery(`/api/v1/clubs/${encodeURIComponent(ref)}/posts/archive`, year ? 'year=' + year : '')),
  // fromClub: the post speaks for the club ("От имени сообщества"),
  // signed: naming you under it ("Подпись автора").
  createClubPost: (ref, text, photos = [], { fromClub = false, signed = false, nsfw = false, poll = null, documents = [] } = {}) =>
    authorized('POST', `/api/v1/clubs/${encodeURIComponent(ref)}/posts`, { text, photos, fromClub, signed, nsfw, documents, ...(poll && { poll }) }),
  // "Предложка": a post suggested for the admins of a club to publish.
  suggestClubPost: (ref, text, photos = []) => authorized('POST', `/api/v1/clubs/${encodeURIComponent(ref)}/suggestions`, { text, photos }),
  // mine: only yours ("Предложенные вами записи"); all of them for the admins.
  clubSuggestions(ref, { mine = false, cursor = '', limit = 0 } = {}) {
    const path = page(`/api/v1/clubs/${encodeURIComponent(ref)}/suggestions`, cursor, limit);
    return authorized('GET', mine ? path + (path.includes('?') ? '&' : '?') + 'mine=true' : path);
  },
  suggestionText: (id) => authorized('GET', `/api/v1/suggestions/${encodeURIComponent(id)}/text`),
  publishSuggestion: (id, text, signed) => authorized('POST', `/api/v1/suggestions/${encodeURIComponent(id)}/publish`, { text, signed }),
  deleteSuggestion: (id) => authorized('DELETE', `/api/v1/suggestions/${encodeURIComponent(id)}`),

  // "Обсуждения": q finds topics by their titles.
  clubTopics(ref, { q = '', cursor = '', limit = 0 } = {}) {
    const path = page(`/api/v1/clubs/${encodeURIComponent(ref)}/topics`, cursor, limit);
    return optional('GET', q ? path + (path.includes('?') ? '&' : '?') + new URLSearchParams({ q }) : path);
  },
  // text and photos are the first message; fromClub speaks for the club.
  createTopic: (ref, title, text, photos = [], { fromClub = false } = {}) =>
    authorized('POST', `/api/v1/clubs/${encodeURIComponent(ref)}/topics`, { title, text, photos, fromClub }),
  topic: (id) => optional('GET', `/api/v1/topics/${encodeURIComponent(id)}`),
  editTopic: (id, { title, closed, pinned, restricted }) =>
    authorized('PATCH', `/api/v1/topics/${encodeURIComponent(id)}`, { title, closed, pinned, restricted }),
  deleteTopic: (id) => authorized('DELETE', `/api/v1/topics/${encodeURIComponent(id)}`),
  topicMessages: (id, cursor = '') => optional('GET', page(`/api/v1/topics/${encodeURIComponent(id)}/messages`, cursor, 0)),
  writeTopicMessage: (id, text, photos = [], { fromClub = false } = {}) =>
    authorized('POST', `/api/v1/topics/${encodeURIComponent(id)}/messages`, { text, photos, fromClub }),
  deleteTopicMessage: (id) => authorized('DELETE', `/api/v1/topic-messages/${encodeURIComponent(id)}`),
  // The heart of a message of a topic, as of a comment.
  topicMessageText: (id) => optional('GET', `/api/v1/topic-messages/${encodeURIComponent(id)}/text`),
  likeTopicMessage: (id) => authorized('PUT', `/api/v1/topic-messages/${encodeURIComponent(id)}/like`),
  unlikeTopicMessage: (id) => authorized('DELETE', `/api/v1/topic-messages/${encodeURIComponent(id)}/like`),
  topicMessageLikers: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/topic-messages/${encodeURIComponent(id)}/likes`, cursor, limit)),

  // "Заметки" of a person, the comments under a note; the list carries
  // the beginnings of the texts, note(id) the whole.
  // "Закладки": what you like — section is 'posts', 'comments' or
  // 'photos' — newest like first, and ownPosts: what you wrote.
  faves: (section, cursor = '') => authorized('GET', page(`/api/v1/me/faves/${section}`, cursor, 0)),
  ownPosts: (cursor = '') => authorized('GET', page('/api/v1/me/posts', cursor, 0)),

  // "Подарки": what there is to send, what a page was sent, sending and
  // removing from your own page.
  giftCatalog: () => request('GET', '/api/v1/gifts'),
  gifts: (nickname, cursor = '', limit = 0) => optional('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/gifts`, cursor, limit)),
  sendGift: (nickname, gift, caption, anonymous) => authorized('POST', `/api/v1/users/${encodeURIComponent(nickname)}/gifts`, { gift, caption, anonymous }),
  deleteGift: (id) => authorized('DELETE', `/api/v1/gifts/${encodeURIComponent(id)}`),
  // Stickers: the "Магазин стикеров", whose packs one adds, free or for
  // votes, and puts away.
  stickers: () => optional('GET', '/api/v1/stickers'),
  addStickerPack: (id) => authorized('PUT', `/api/v1/me/stickers/${encodeURIComponent(id)}`),
  removeStickerPack: (id) => authorized('DELETE', `/api/v1/me/stickers/${encodeURIComponent(id)}`),
  // Votes ("голоса"): a voucher gives them, a transfer hands them over;
  // the vouchers themselves are the administrators'.
  redeemVoucher: (code) => authorized('POST', '/api/v1/me/vouchers', { code }),
  transferVotes: (nickname, votes) => authorized('POST', `/api/v1/users/${encodeURIComponent(nickname)}/votes`, { votes }),
  vouchers: (cursor = '') => authorized('GET', page('/api/v1/vouchers', cursor, 0)),
  createVoucher: (voucher) => authorized('POST', '/api/v1/vouchers', voucher),
  deleteVoucher: (id) => authorized('DELETE', `/api/v1/vouchers/${encodeURIComponent(id)}`),

  notes: (nickname, cursor = '', limit = 0) => optional('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/notes`, cursor, limit)),
  createNote: (title, text) => authorized('POST', '/api/v1/me/notes', { title, text }),
  note: (id) => optional('GET', `/api/v1/notes/${encodeURIComponent(id)}`),
  editNote: (id, title, text) => authorized('PATCH', `/api/v1/notes/${encodeURIComponent(id)}`, { title, text }),
  deleteNote: (id) => authorized('DELETE', `/api/v1/notes/${encodeURIComponent(id)}`),
  noteComments: (id, cursor = '') => optional('GET', page(`/api/v1/notes/${encodeURIComponent(id)}/comments`, cursor, 0)),
  commentNote: (id, text) => authorized('POST', `/api/v1/notes/${encodeURIComponent(id)}/comments`, { text }),
  deleteNoteComment: (id) => authorized('DELETE', `/api/v1/note-comments/${encodeURIComponent(id)}`),
  noteCommentText: (id) => optional('GET', `/api/v1/note-comments/${encodeURIComponent(id)}/text`),

  // "Альбомы" of a person (nickname) or of a community (club: its ref).
  albums({ nickname = '', club = '', cursor = '', limit = 0 } = {}) {
    const owner = club ? `/api/v1/clubs/${encodeURIComponent(club)}` : `/api/v1/users/${encodeURIComponent(nickname)}`;
    return optional('GET', page(owner + '/albums', cursor, limit));
  },
  // club: an album of the community, its admins only; yours without it.
  createAlbum: (name, description, { club = '' } = {}) =>
    authorized('POST', club ? `/api/v1/clubs/${encodeURIComponent(club)}/albums` : '/api/v1/me/albums', { name, description }),
  album: (id) => optional('GET', `/api/v1/albums/${encodeURIComponent(id)}`),
  editAlbum: (id, name, description) => authorized('PATCH', `/api/v1/albums/${encodeURIComponent(id)}`, { name, description }),
  deleteAlbum: (id) => authorized('DELETE', `/api/v1/albums/${encodeURIComponent(id)}`),
  albumPhotos: (id, cursor = '') => optional('GET', page(`/api/v1/albums/${encodeURIComponent(id)}/photos`, cursor, 0)),
  // photos are [{ id, description }] uploaded with uploadPhoto, up to 10.
  addAlbumPhotos: (id, photos) => authorized('POST', `/api/v1/albums/${encodeURIComponent(id)}/photos`, { photos }),
  albumPhoto: (id) => optional('GET', `/api/v1/album-photos/${encodeURIComponent(id)}`),
  describeAlbumPhoto: (id, description) => authorized('PATCH', `/api/v1/album-photos/${encodeURIComponent(id)}`, { description }),
  deleteAlbumPhoto: (id) => authorized('DELETE', `/api/v1/album-photos/${encodeURIComponent(id)}`),
  // The photo that shows an album instead of its first, and a photo put in
  // another album of the same owner: whoever may edit the albums.
  setAlbumCover: (id, photoId) => authorized('PUT', `/api/v1/albums/${encodeURIComponent(id)}/cover`, { photoId }),
  moveAlbumPhoto: (id, albumId) => authorized('PUT', `/api/v1/album-photos/${encodeURIComponent(id)}/album`, { albumId }),
  // The heart, likers, comments and "Поделиться" of a photo, as of a post.
  likePhoto: (id) => authorized('PUT', `/api/v1/album-photos/${encodeURIComponent(id)}/like`),
  unlikePhoto: (id) => authorized('DELETE', `/api/v1/album-photos/${encodeURIComponent(id)}/like`),
  photoLikers: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/album-photos/${encodeURIComponent(id)}/likes`, cursor, limit)),
  photoCommentText: (id) => optional('GET', `/api/v1/photo-comments/${encodeURIComponent(id)}/text`),
  photoComments: (id, cursor = '') => optional('GET', page(`/api/v1/album-photos/${encodeURIComponent(id)}/comments`, cursor, 0)),
  // replyTo: the comment under the same photo this one answers.
  commentPhoto: (id, text, replyTo = '') => authorized('POST', `/api/v1/album-photos/${encodeURIComponent(id)}/comments`, { text, replyTo }),
  likePhotoComment: (id) => authorized('PUT', `/api/v1/photo-comments/${encodeURIComponent(id)}/like`),
  unlikePhotoComment: (id) => authorized('DELETE', `/api/v1/photo-comments/${encodeURIComponent(id)}/like`),
  photoCommentLikers: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/photo-comments/${encodeURIComponent(id)}/likes`, cursor, limit)),
  deletePhotoComment: (id) => authorized('DELETE', `/api/v1/photo-comments/${encodeURIComponent(id)}`),
  sharePhoto: (id, text, to = {}) => authorized('POST', `/api/v1/album-photos/${encodeURIComponent(id)}/reposts`, { text, ...to }),

  // filters are those openvk's page links to: { hometown, city, film,
  // show, book }; with one, query may be empty.
  search(query, cursor = '', filters = {}) {
    const params = new URLSearchParams({ q: query, ...filters });
    if (cursor) params.set('cursor', cursor);
    return request('GET', '/api/v1/users?' + params);
  },
};

// withType adds the tab of a wall to its path; comments: its posts come
// with their newest comments, as "Микроблог" shows them.
function withType(path, type, q = '', comments = false, year = 0) {
  for (const [name, value] of [['type', type], ['q', q], ['comments', comments ? 'last' : ''], ['year', year || '']]) {
    if (value) path += (path.includes('?') ? '&' : '?') + name + '=' + encodeURIComponent(value);
  }

  return path;
}

// feedPath is the request of a page of the news with its settings; what
// is as the server has it by default is not said.
function feedPath(cursor, comments, all, { limit = 0, alien = true, ignored = false }) {
  let path = withType(page('/api/v1/feed', cursor, limit), all ? 'all' : '', '', comments);
  if (!alien) path = withQuery(path, 'alien=false');
  if (ignored) path = withQuery(path, 'ignored=true');

  return path;
}

// withQuery adds param ("name=value"), if any, to the query of path.
function withQuery(path, param) {
  return param ? path + (path.includes('?') ? '&' : '?') + param : path;
}

function page(path, cursor, limit) {
  const query = new URLSearchParams();
  if (cursor) query.set('cursor', cursor);
  if (limit) query.set('limit', String(limit));
  return query.size ? `${path}?${query}` : path;
}

// currentUser is the signed-in user, or null when signed out or the
// session is no longer valid.
export async function currentUser() {
  if (!session.get()) return null;

  try {
    return await api.me();
  } catch (e) {
    if (e.status === 401 || e.status === 404) {
      session.clear();
      return null;
    }
    throw e;
  }
}
