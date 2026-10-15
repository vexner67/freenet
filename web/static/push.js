// Push notifications on this device: the browser subscribes with its push
// service, and notification sends there what would appear in "Мои Ответы"
// and that a message came (sw.js shows them). One browser pushes to one
// account: whoever turned them on last, or signed in last, here.
//
// localStorage keeps whether they are on here (freenet.push), and for
// whom and which subscription the server knows, so that each page does
// not tell it again.

import { api } from './api.js';

const KEY = 'freenet.push';

function stored() {
  try {
    return JSON.parse(localStorage.getItem(KEY)) ?? {};
  } catch {
    return {};
  }
}

function store(state) {
  try {
    localStorage.setItem(KEY, JSON.stringify(state));
  } catch {
    // Not kept: the next page tells the server again.
  }
}

export function pushSupported() {
  return typeof navigator !== 'undefined' && 'serviceWorker' in navigator &&
    typeof window !== 'undefined' && 'PushManager' in window && 'Notification' in window;
}

// needsInstall is an iPhone's: pushes reach only a site added to the home
// screen and opened from there.
export function needsInstall() {
  const apple = /iPhone|iPad|iPod/.test(navigator.userAgent) ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
  return apple && !navigator.standalone && !matchMedia('(display-mode: standalone)').matches;
}

// keyBytes reads the base64url public key of the server.
export function keyBytes(key) {
  const base64 = key.replaceAll('-', '+').replaceAll('_', '/');
  return Uint8Array.from(atob(base64 + '='.repeat((4 - base64.length % 4) % 4)), (c) => c.charCodeAt(0));
}

function sameKey(subscription, key) {
  const used = subscription.options?.applicationServerKey;
  if (!used) return true;

  const a = new Uint8Array(used);
  return a.length === key.length && a.every((b, i) => b === key[i]);
}

// subscription is the browser's, made with the key of the server: one
// made with a key the server no longer has is made anew.
async function subscription(serverKey) {
  const registration = await navigator.serviceWorker.register('/sw.js');
  await navigator.serviceWorker.ready;

  const key = keyBytes(serverKey);
  const had = await registration.pushManager.getSubscription();
  if (had && sameKey(had, key)) return had;
  if (had) await had.unsubscribe();

  return registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
}

async function tell(me, serverKey) {
  const made = await subscription(serverKey);
  await api.subscribePush(made.toJSON());
  store({ on: true, user: me.id, endpoint: made.endpoint, at: Date.now() });
}

// pushState is what the settings say: 'unsupported', 'install' (an iPhone
// that has to add the site to the home screen first), 'off-server' (the
// site sends none), 'denied', 'on' or 'off'.
export async function pushState() {
  if (!pushSupported()) return needsInstall() ? 'install' : 'unsupported';

  const { publicKey } = await api.pushKey();
  if (!publicKey) return 'off-server';
  if (Notification.permission === 'denied') return 'denied';

  return stored().on && Notification.permission === 'granted' ? 'on' : 'off';
}

// enablePush asks the browser for permission (on a click: browsers ask
// only then) and subscribes me; it tells whether they are on now. The
// permission is asked first: Safari asks only straight in the tap, not
// after a request to the server.
export async function enablePush(me) {
  if (await Notification.requestPermission() !== 'granted') return false;

  const { publicKey } = await api.pushKey();
  if (!publicKey) return false;

  await tell(me, publicKey);
  return true;
}

// disablePush forgets the subscription here and at the server.
export async function disablePush() {
  store({ on: false });

  const registration = await navigator.serviceWorker.getRegistration('/');
  const had = await registration?.pushManager.getSubscription();
  if (!had) return;

  await api.unsubscribePush(had.endpoint).catch(() => {});
  await had.unsubscribe();
}

// A day: the server is told again now and then, in case it forgot the
// subscription.
const AGAIN = 24 * 60 * 60 * 1000;

// syncPush tells the server about this browser when the account signed in
// is not the one it pushes to, or the subscription changed.
export async function syncPush(me) {
  const state = stored();
  if (!state.on || !pushSupported() || Notification.permission !== 'granted') return;

  const registration = await navigator.serviceWorker.getRegistration('/');
  const had = await registration?.pushManager.getSubscription();
  if (had && state.user === me.id && state.endpoint === had.endpoint && Date.now() - (state.at ?? 0) < AGAIN) return;

  const { publicKey } = await api.pushKey();
  if (publicKey) await tell(me, publicKey);
}

// closePushes takes away the pushes this browser shows whose tags start
// so (sw.js: 'im:<user>', 'chat:<chat>', 'n:<notification>'): what was
// read here is no longer news in the notification shade of a phone.
export async function closePushes(...prefixes) {
  if (!pushSupported()) return;

  const registration = await navigator.serviceWorker.getRegistration('/');
  for (const n of await registration?.getNotifications() ?? []) {
    if (prefixes.some((p) => n.tag.startsWith(p))) n.close();
  }
}

// showUnread puts the number of what is unread on the icon of the site
// added to the home screen (the Badging API; elsewhere it does nothing),
// and tells the service worker, which counts on from it as pushes come.
export function showUnread(n) {
  if (typeof navigator === 'undefined' || !('setAppBadge' in navigator)) return;

  (n > 0 ? navigator.setAppBadge(n) : navigator.clearAppBadge()).catch(() => {});
  navigator.serviceWorker?.controller?.postMessage({ unread: n });
}

// onPhone is a narrow screen or the site opened from the home screen.
export function onPhone() {
  return matchMedia('(max-width: 800px)').matches || matchMedia('(display-mode: standalone)').matches ||
    navigator.standalone === true;
}

const OFFER_KEY = 'freenet.pushoffer';

function offered() {
  try {
    return localStorage.getItem(OFFER_KEY) !== null;
  } catch {
    // Without storage a bar on every page would be a nuisance.
    return true;
  }
}

// pushOffer is what a phone is offered over its pages, once: 'enable'
// when a tap turns pushes on, 'install' on an iPhone that has to add the
// site to the home screen first; null when nothing is (not a phone,
// declined already, on already, or decided in the browser).
export async function pushOffer() {
  if (!onPhone() || offered()) return null;
  if (!pushSupported()) return needsInstall() ? 'install' : null;
  if (stored().on || Notification.permission !== 'default') return null;

  const { publicKey } = await api.pushKey();
  return publicKey ? 'enable' : null;
}

// endPushOffer keeps the offer from coming back, whatever was answered.
export function endPushOffer() {
  try {
    localStorage.setItem(OFFER_KEY, String(Date.now()));
  } catch {
    // offered() says so without storage anyway.
  }
}

// forgetPushAccount stops pushing to the account signing out: the next
// one to sign in here gets the pushes, if they are on.
export async function forgetPushAccount() {
  const state = stored();
  if (!state.user || !pushSupported()) return;

  store({ on: state.on });

  const registration = await navigator.serviceWorker.getRegistration('/');
  const had = await registration?.pushManager.getSubscription();
  if (had) await api.unsubscribePush(had.endpoint).catch(() => {});
}
