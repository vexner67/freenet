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
  updatePrivacy: (privacy) => authorized('PUT', '/api/v1/me/privacy', privacy),
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
  wall: (nickname, cursor = '') => optional('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/posts`, cursor, 0)),
  post: (id) => optional('GET', '/api/v1/posts/' + encodeURIComponent(id)),
  // Writes on the wall of nickname: someone else's or your own.
  createPost: (nickname, text, photos = []) => authorized('POST', `/api/v1/users/${encodeURIComponent(nickname)}/posts`, { text, photos }),
  // Uploads a photo for a post; the post refers to it by ID.
  uploadPhoto: (file) => authorized('POST', '/api/v1/me/photos', file),
  // Shares a post on your wall, with a comment that may be empty.
  repost: (postID, text) => authorized('POST', `/api/v1/posts/${encodeURIComponent(postID)}/reposts`, { text }),
  deletePost: (id) => authorized('DELETE', '/api/v1/posts/' + encodeURIComponent(id)),
  like: (postID) => authorized('PUT', `/api/v1/posts/${encodeURIComponent(postID)}/like`),
  unlike: (postID) => authorized('DELETE', `/api/v1/posts/${encodeURIComponent(postID)}/like`),

  comments: (postID, cursor = '') => request('GET', page(`/api/v1/posts/${encodeURIComponent(postID)}/comments`, cursor, 0)),
  createComment: (postID, text) => authorized('POST', `/api/v1/posts/${encodeURIComponent(postID)}/comments`, { text }),
  deleteComment: (id) => authorized('DELETE', '/api/v1/comments/' + encodeURIComponent(id)),

  follow: (nickname) => authorized('POST', `/api/v1/users/${encodeURIComponent(nickname)}/follow`),
  unfollow: (nickname) => authorized('DELETE', `/api/v1/users/${encodeURIComponent(nickname)}/follow`),
  // The blacklist ("Чёрный список"); blocking drops the follows both ways.
  block: (nickname) => authorized('PUT', `/api/v1/me/blacklist/${encodeURIComponent(nickname)}`),
  unblock: (nickname) => authorized('DELETE', `/api/v1/me/blacklist/${encodeURIComponent(nickname)}`),
  blacklist: (cursor = '') => authorized('GET', page('/api/v1/me/blacklist', cursor, 0)),
  followers: (nickname, cursor = '', limit = 0) => request('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/followers`, cursor, limit)),
  following: (nickname, cursor = '', limit = 0) => request('GET', page(`/api/v1/users/${encodeURIComponent(nickname)}/following`, cursor, limit)),
  feed: (cursor = '') => authorized('GET', page('/api/v1/feed', cursor, 0)),

  uploadAvatar: (file) => authorized('PUT', '/api/v1/me/avatar', file),
  deleteAvatar: () => authorized('DELETE', '/api/v1/me/avatar'),

  notifications(box, cursor = '') {
    const query = new URLSearchParams({ box });
    if (cursor) query.set('cursor', cursor);
    return authorized('GET', '/api/v1/notifications?' + query);
  },
  unreadNotifications: () => authorized('GET', '/api/v1/notifications/unread-count'),
  markNotificationsRead: (upTo) => authorized('POST', '/api/v1/notifications/read', { upTo }),

  dialogs: (cursor = '') => authorized('GET', page('/api/v1/dialogs', cursor, 0)),
  unreadDialogs: () => authorized('GET', '/api/v1/dialogs/unread-count'),
  // Older messages with cursor, or newer ones with after (polling).
  messages(nickname, { cursor = '', after = '' } = {}) {
    const query = new URLSearchParams();
    if (cursor) query.set('cursor', cursor);
    if (after) query.set('after', after);
    const path = `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages`;
    return authorized('GET', query.size ? `${path}?${query}` : path);
  },
  sendMessage: (nickname, text) => authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/messages`, { text }),
  markDialogRead: (nickname, upTo) => authorized('POST', `/api/v1/dialogs/${encodeURIComponent(nickname)}/read`, { upTo }),

  search(query, cursor = '') {
    const params = new URLSearchParams({ q: query });
    if (cursor) params.set('cursor', cursor);
    return request('GET', '/api/v1/users?' + params);
  },
};

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
