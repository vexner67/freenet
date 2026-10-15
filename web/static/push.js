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
// only then) and subscribes me; it tells whether they are on now.
export async function enablePush(me) {
  const { publicKey } = await api.pushKey();
  if (!publicKey) return false;
  if (await Notification.requestPermission() !== 'granted') return false;

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
