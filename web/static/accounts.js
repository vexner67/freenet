// Several accounts in one browser, as on VK: besides the account that is
// signed in, the browser keeps the sessions of those set aside ("parked"),
// and switching swaps one for another without signing out and in. The
// tokens live in localStorage next to those of the session (api.js).

import { exclusive, readAs, renewTokens, session } from './api.js';

const KEY = 'freenet.accounts';
// CURRENT_KEY names the account signed in, for the other tabs to hear
// that it has changed.
const CURRENT_KEY = 'freenet.account';

// How often a page looks after the accounts that wait: their tokens,
// names and unread counters.
const TENDED_EVERY = 2 * 60 * 1000;
// An access token this close to its end is replaced.
const EXPIRES_SOON = 60 * 1000;

// MAX_PARKED is how many accounts wait besides the one signed in.
export const MAX_PARKED = 4;

// accountOf is an account set aside: its session and what the menu shows
// of whose it is.
export function accountOf(me, tokens) {
  return {
    id: me.id,
    name: me.name ?? '',
    surname: me.surname ?? '',
    nickname: me.nickname ?? '',
    avatar: me.avatar?.small ?? '',
    accessToken: tokens.accessToken,
    refreshToken: tokens.refreshToken,
  };
}

// accountName is how the menu calls an account: by name or, before the
// page is filled in, by nothing better than "Новая страница".
export function accountName(account) {
  return [account.name, account.surname].filter(Boolean).join(' ') || 'Новая страница';
}

// kept puts account first among the parked, once.
export function kept(parked, account) {
  return [account, ...left(parked, account.id)].slice(0, MAX_PARKED + 1);
}

// left is the parked without the account with id.
export function left(parked, id) {
  return parked.filter((a) => a.id !== id);
}

// swapped is what a switch to the parked account with id leaves: its
// tokens to sign in with and the parked, with current, when there is
// one, in its place. null when no such account waits.
export function swapped(parked, id, current) {
  const target = parked.find((a) => a.id === id);
  if (!target) return null;

  const rest = left(parked, id);

  return {
    tokens: { accessToken: target.accessToken, refreshToken: target.refreshToken },
    parked: current ? kept(rest, current) : rest,
  };
}

// expiresAt is when an access token ends, in milliseconds; 0 for one
// that does not say.
export function expiresAt(accessToken) {
  try {
    const payload = accessToken.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');

    return (JSON.parse(atob(payload)).exp ?? 0) * 1000;
  } catch {
    return 0;
  }
}

// isDue: it is time to look after a parked account, at now.
export function isDue(account, now) {
  return !(now - (account.tendedAt ?? 0) < TENDED_EVERY) || expiresAt(account.accessToken) - now < EXPIRES_SOON;
}

// tended is the parked with what was learnt about the account with id
// put into it; an account that waits no more is not brought back.
export function tended(parked, id, learnt) {
  return parked.map((a) => (a.id === id ? { ...a, ...learnt } : a));
}

function valid(a) {
  return Boolean(a && typeof a.id === 'string' && typeof a.accessToken === 'string' && typeof a.refreshToken === 'string');
}

// parkedAccounts are the accounts set aside in this browser.
export function parkedAccounts() {
  try {
    const list = JSON.parse(localStorage.getItem(KEY));

    return Array.isArray(list) ? list.filter(valid) : [];
  } catch {
    return [];
  }
}

function save(parked) {
  if (parked.length > 0) localStorage.setItem(KEY, JSON.stringify(parked));
  else localStorage.removeItem(KEY);
}

// canPark: there is room to set one more account aside.
export function canPark() {
  return parkedAccounts().length < MAX_PARKED;
}

// current is the account signed in, as it would wait.
function current(me) {
  const tokens = session.get();

  return me && tokens ? accountOf(me, tokens) : null;
}

// park sets the account of me aside and leaves the browser signed out,
// for another account to sign in ("Добавить аккаунт"). It runs under the
// lock of the refresh: a refresh in flight would write the tokens of the
// account set aside back.
export function park(me) {
  return exclusive(() => {
    const account = current(me);
    if (!account) return;

    save(kept(parkedAccounts(), account));
    session.clear();
    signedInAs('');
  });
}

// switchTo signs in as the parked account with id, setting the account
// of me, when someone is signed in, aside. It tells whether it did.
export function switchTo(id, me = null) {
  return exclusive(() => {
    const result = swapped(parkedAccounts(), id, current(me));
    if (!result) return false;

    save(result.parked);
    session.save(result.tokens);
    signedInAs(id);

    return true;
  });
}

// settle runs once me has signed in: an account that waited has a newer
// session now and waits no more.
export function settle(me) {
  const parked = parkedAccounts();
  if (parked.some((a) => a.id === me.id)) save(left(parked, me.id));
}

// signedInAs tells the other tabs whose the session is now, '' for no
// one's: they call back what onAccountChange was given.
export function signedInAs(id) {
  try {
    if (localStorage.getItem(CURRENT_KEY) === (id || null)) return;

    if (id) localStorage.setItem(CURRENT_KEY, id);
    else localStorage.removeItem(CURRENT_KEY);
  } catch {
    // Without storage there are no other tabs to tell.
  }
}

// onAccountChange calls back when another tab switched accounts or
// signed out: this page shows what is not its account's anymore.
export function onAccountChange(callback) {
  window.addEventListener('storage', (event) => {
    if (event.key === CURRENT_KEY) callback();
  });
}

// renewed replaces the tokens of the parked account with id when they
// are about to end, so that an account that waits does not lose its
// session: a refresh token lives only until the next refresh is due. A
// session that is over leaves the list. It answers the account as it is
// now, null when it waits no more.
function renewed(id) {
  return exclusive(async () => {
    const account = parkedAccounts().find((a) => a.id === id);
    if (!account) return null;
    if (expiresAt(account.accessToken) - Date.now() >= EXPIRES_SOON) return account;

    try {
      const { accessToken, refreshToken } = await renewTokens(account.refreshToken);

      save(tended(parkedAccounts(), id, { accessToken, refreshToken }));

      return { ...account, accessToken, refreshToken };
    } catch (e) {
      if (e.status === 401) save(left(parkedAccounts(), id));

      return null;
    }
  });
}

// learn asks what the menu shows of a parked account: whose it is and
// how much waits there unread.
async function learn(account) {
  const read = (path) => readAs(account.accessToken, path);
  const [me, messages, notifications] = await Promise.all([
    read('/api/v1/me'),
    read('/api/v1/dialogs/unread-count').catch(() => null),
    read('/api/v1/notifications/unread-count').catch(() => null),
  ]);

  return {
    name: me.name ?? '',
    surname: me.surname ?? '',
    nickname: me.nickname ?? '',
    avatar: me.avatar?.small ?? '',
    unread: (messages?.count ?? 0) + (notifications?.count ?? 0),
  };
}

// tendParked looks after the accounts that wait, those it is time for:
// keeps their sessions alive and learns their names, photos and unread
// counters. It tells whether anything changed. One tab at a time, the
// others find the accounts looked after.
export async function tendParked() {
  let changed = false;

  for (const { id } of parkedAccounts()) {
    const now = Date.now();
    if (!isDue(parkedAccounts().find((a) => a.id === id) ?? {}, now)) continue;

    try {
      const account = await renewed(id);
      if (!account) {
        changed = true;
        continue;
      }

      const learnt = { ...await learn(account), tendedAt: now };

      await exclusive(() => save(tended(parkedAccounts(), id, learnt)));
      changed = true;
    } catch {
      // The next page tries again.
    }
  }

  return changed;
}
