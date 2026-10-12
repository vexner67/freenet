// Several accounts in one browser, as on VK: besides the account that is
// signed in, the browser keeps the sessions of those set aside ("parked"),
// and switching swaps one for another without signing out and in. The
// tokens live in localStorage next to those of the session (api.js).

import { exclusive, session } from './api.js';

const KEY = 'freenet.accounts';

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

    return true;
  });
}

// settle runs once me has signed in: an account that waited has a newer
// session now and waits no more.
export function settle(me) {
  const parked = parkedAccounts();
  if (parked.some((a) => a.id === me.id)) save(left(parked, me.id));
}

// onAccountsChange calls back when another tab switched accounts: this
// page shows what is not its account's anymore.
export function onAccountsChange(callback) {
  window.addEventListener('storage', (event) => {
    if (event.key === KEY) callback();
  });
}
