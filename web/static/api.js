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

  if (navigator.locks) {
    await navigator.locks.request('freenet-refresh', run);
  } else {
    await run();
  }
}

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
  updateProfile: (profile) => authorized('PUT', '/api/v1/me/profile', profile),
  // postLikers and commentLikers are openvk's "Понравилось N людям".
  postLikers: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/posts/${encodeURIComponent(id)}/likes`, cursor, limit)),
  commentLikers: (id, cursor = '', limit = 0) => optional('GET', page(`/api/v1/comments/${encodeURIComponent(id)}/likes`, cursor, limit)),
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
  wall: (nickname, cursor = '', type = '', q = '', comments = false) => optional('GET',
    withType(page(`/api/v1/users/${encodeURIComponent(nickname)}/posts`, cursor, 0), type, q, comments)),
  post: (id) => optional('GET', '/api/v1/posts/' + encodeURIComponent(id)),
  // Writes on the wall of nickname: someone else's or your own.
  // nsfw marks the post as adult ("Содержит NSFW-контент").
  createPost: (nickname, text, photos = [], nsfw = false) => authorized('POST', `/api/v1/users/${encodeURIComponent(nickname)}/posts`, { text, photos, nsfw }),
  // Uploads a photo for a post; the post refers to it by ID.
  uploadPhoto: (file) => authorized('POST', '/api/v1/me/photos', file),
  // Shares a post on your wall, with a comment that may be empty.
  // to is { club, fromClub, signed } to share it in a community ("в группу").
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
  feed: (cursor = '', comments = false, all = false) => authorized('GET', withType(page('/api/v1/feed', cursor, 0), all ? 'all' : '', '', comments)),

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

  dialogs: (cursor = '') => authorized('GET', page('/api/v1/dialogs', cursor, 0)),
  unreadDialogs: () => authorized('GET', '/api/v1/dialogs/unread-count'),
  // Older messages with cursor, newer ones with after (polling), or the
  // part of the dialog around a message, as a found one opens.
  messages(nickname, { cursor = '', after = '', around = '' } = {}) {
    const query = new URLSearchParams();
    if (cursor) query.set('cursor', cursor);
    if (after) query.set('after', after);
    if (around) query.set('around', around);
    const path = `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages`;
    return authorized('GET', query.size ? `${path}?${query}` : path);
  },
  // post shares a post into the dialog ("Поделиться" "в переписку");
  // forward carries messages of your dialogs on ("Переслать").
  sendMessage: (nickname, text, photos = [], post = '', forward = []) =>
    authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages`, { text, photos, post, forward }),
  markDialogRead: (nickname, upTo) => authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/read`, { upTo }),
  // Your own message, from both sides, within a day of sending: it stays
  // in the dialog as "Сообщение удалено"; forMe
  // deletes any message of your dialog for you alone ("Удалить у себя").
  deleteMessage: (id, { forMe = false } = {}) => authorized('DELETE', `/api/v1/messages/${encodeURIComponent(id)}` + (forMe ? '?for=me' : '')),
  // What your own message says, within a day of sending.
  // message is a message of a dialog whole: the dialog carries the
  // beginnings of long texts.
  message: (nickname, id) => authorized('GET', `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages/${encodeURIComponent(id)}`),
  editMessage: (id, text) => authorized('PATCH', `/api/v1/messages/${encodeURIComponent(id)}`, { text }),
  // Your messages that say q, newest first: with peer, a nickname, in
  // that dialog only.
  searchMessages(q, { peer = '', cursor = '' } = {}) {
    const query = new URLSearchParams({ q });
    if (peer) query.set('peer', peer);
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
  appointAdmin: (ref, nickname) => authorized('PUT', `/api/v1/clubs/${encodeURIComponent(ref)}/admins/${encodeURIComponent(nickname)}`),
  dismissAdmin: (ref, nickname) => authorized('DELETE', `/api/v1/clubs/${encodeURIComponent(ref)}/admins/${encodeURIComponent(nickname)}`),
  // Those who run a club give one of them a title and hide or show them
  // ("Изменить комментарий", "Скрыт").
  describeAdmin: (ref, nickname, { title, hidden }) =>
    authorized('PATCH', `/api/v1/clubs/${encodeURIComponent(ref)}/admins/${encodeURIComponent(nickname)}`, { title, hidden }),
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
  clubWall: (ref, cursor = '', type = '', q = '', comments = false) => optional('GET',
    withType(page(`/api/v1/clubs/${encodeURIComponent(ref)}/posts`, cursor, 0), type, q, comments)),
  // fromClub: the post speaks for the club ("От имени сообщества"),
  // signed: naming you under it ("Подпись автора").
  createClubPost: (ref, text, photos = [], { fromClub = false, signed = false, nsfw = false } = {}) =>
    authorized('POST', `/api/v1/clubs/${encodeURIComponent(ref)}/posts`, { text, photos, fromClub, signed, nsfw }),
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
  commentPhoto: (id, text) => authorized('POST', `/api/v1/album-photos/${encodeURIComponent(id)}/comments`, { text }),
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
function withType(path, type, q = '', comments = false) {
  for (const [name, value] of [['type', type], ['q', q], ['comments', comments ? 'last' : '']]) {
    if (value) path += (path.includes('?') ? '&' : '?') + name + '=' + encodeURIComponent(value);
  }

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
