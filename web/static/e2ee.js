// End-to-end encryption of the messages of dialogs. Each user has a key
// pair (X25519) made by their browser: the server keeps the public key
// only, the private one lives in the browsers of its owner, moved from one
// to another as a file locked with a code (exportKeys, importKeys). What a
// message says, its words and the words of the messages it carries on, is
// sealed for the two people of the dialog: AES-256-GCM with a key both of
// them derive (HKDF-SHA256 of the X25519 shared secret). The sealed bytes
// name the public keys of the sender and of the recipient, so either side
// opens them with whichever of its keys was used, an older one too.
// Messages of chats are not sealed.

import { api } from './api.js';

const KEYS = 'freenet.keys';
const PEERS = 'freenet.peerkeys';
const VERSION = 1;
const KEY_SIZE = 32;
const IV_SIZE = 12;
const HEADER_SIZE = 1 + 2 * KEY_SIZE + IV_SIZE;
const INFO = 'freenet message v1';
// How often a page asks whether the key here is still the one the server
// has: another browser may have set up a new one.
const CHECK_MS = 5 * 60 * 1000;
// The file a key moves in: PBKDF2 with this many rounds over the code.
const ROUNDS = 310000;
// The code that locks the file: no letters to confuse (0/O, 1/I/L).
const CODE_ALPHABET = '23456789ABCDEFGHJKMNPQRSTUVWXYZ';
const CODE_GROUPS = 5;
const CODE_GROUP = 4;

const encoder = new TextEncoder();
const decoder = new TextDecoder();

// The states of the key of this browser: ready — it is the one the server
// has; missing — the server has a key this browser does not, set up in
// another one; stale — this browser has an older one.
export const KEY_READY = 'ready';
export const KEY_MISSING = 'missing';
export const KEY_STALE = 'stale';

export function toBase64(bytes) {
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);

  return btoa(binary);
}

export function fromBase64(text) {
  return Uint8Array.from(atob(text), (c) => c.charCodeAt(0));
}

function concat(...parts) {
  const all = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let at = 0;

  for (const p of parts) {
    all.set(p, at);
    at += p.length;
  }

  return all;
}

// compareBytes orders keys as the server and the other side do.
function compareBytes(a, b) {
  for (let i = 0; i < Math.min(a.length, b.length); i++) {
    if (a[i] !== b[i]) return a[i] - b[i];
  }

  return a.length - b.length;
}

function read(name) {
  try {
    return JSON.parse(localStorage.getItem(name)) ?? {};
  } catch {
    return {};
  }
}

function write(name, value) {
  try {
    localStorage.setItem(name, JSON.stringify(value));
  } catch {
    // A browser that keeps nothing asks the server again next time.
  }
}

// mine is what this browser keeps for a user: their key pairs, newest
// first ({ private: pkcs8, public: raw }, base64), and what the server
// said of them last.
function mine(userID) {
  const entry = read(KEYS)[userID] ?? {};

  return { keys: entry.keys ?? [], state: entry.state ?? '', checkedAt: entry.checkedAt ?? 0 };
}

function keep(userID, entry) {
  const all = read(KEYS);

  all[userID] = entry;
  write(KEYS, all);
}

// keyState is what this browser last learnt of its key for userID; ''
// before it asked.
export function keyState(userID) {
  return mine(userID).state;
}

// hasKeys tells whether this browser keeps any key of userID.
export function hasKeys(userID) {
  return mine(userID).keys.length > 0;
}

async function makePair() {
  const pair = await crypto.subtle.generateKey({ name: 'X25519' }, true, ['deriveBits']);
  const [pkcs8, raw] = await Promise.all([
    crypto.subtle.exportKey('pkcs8', pair.privateKey),
    crypto.subtle.exportKey('raw', pair.publicKey),
  ]);

  return { private: toBase64(new Uint8Array(pkcs8)), public: toBase64(new Uint8Array(raw)) };
}

// settle records what the server's key means for the keys kept here: the
// one it has comes first.
function settle(userID, keys, server) {
  const index = keys.findIndex((k) => k.public === server);
  let state = KEY_READY;

  if (index > 0) keys.unshift(...keys.splice(index, 1));
  if (index < 0) state = keys.length > 0 ? KEY_STALE : KEY_MISSING;

  keep(userID, { keys, state, checkedAt: Date.now() });
  return state;
}

// ensureKey makes sure userID can seal and open messages here: a user the
// server knows no key of gets one made by this browser; otherwise it says
// whether this browser has the server's key. It asks at most every few
// minutes, unless forced.
export async function ensureKey(userID, { force = false } = {}) {
  const entry = mine(userID);
  if (!force && entry.state && Date.now() - entry.checkedAt < CHECK_MS) return entry.state;

  return locked(() => check(userID));
}

// locked runs one change of the keys at a time, across the tabs too: two
// pages making a key at once would leave the server with the one this
// browser threw away.
function locked(run) {
  return navigator.locks ? navigator.locks.request('freenet.keys', run) : run();
}

async function check(userID) {
  const entry = mine(userID);
  const { publicKey } = await api.myMessageKey();
  if (publicKey) return settle(userID, entry.keys, publicKey);

  // None yet: the one kept here, or a new one, becomes it.
  const keys = entry.keys.length > 0 ? entry.keys : [await makePair()];

  keep(userID, { keys, state: '', checkedAt: 0 });
  await api.setMessageKey(keys[0].public);

  return settle(userID, keys, keys[0].public);
}

// newKey sets up a new key for userID, made here: messages are sealed with
// it from now on. The keys kept before stay, so what they sealed opens.
export function newKey(userID) {
  return locked(() => replaceKey(userID));
}

async function replaceKey(userID) {
  const pair = await makePair();
  const keys = [pair, ...mine(userID).keys];

  keep(userID, { keys, state: '', checkedAt: 0 });
  await api.setMessageKey(pair.public);

  return settle(userID, keys, pair.public);
}

// forgetKeys removes the keys of userID from this browser: messages sealed
// with them do not open here anymore.
export function forgetKeys(userID) {
  const all = read(KEYS);

  delete all[userID];
  write(KEYS, all);
}

// newCode is the code that locks a file with keys: 20 characters, about
// 99 bits.
export function newCode() {
  const random = crypto.getRandomValues(new Uint32Array(CODE_GROUPS * CODE_GROUP));
  const chars = [...random].map((n) => CODE_ALPHABET[n % CODE_ALPHABET.length]);
  const groups = [];

  for (let i = 0; i < chars.length; i += CODE_GROUP) groups.push(chars.slice(i, i + CODE_GROUP).join(''));

  return groups.join('-');
}

// normalCode reads a code as it may be typed: any case, with or without
// dashes and spaces.
export function normalCode(code) {
  return code.toUpperCase().replace(/[^0-9A-Z]/g, '');
}

async function codeKey(code, salt) {
  const base = await crypto.subtle.importKey('raw', encoder.encode(normalCode(code)), 'PBKDF2', false, ['deriveKey']);

  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', hash: 'SHA-256', salt, iterations: ROUNDS },
    base,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt'],
  );
}

// exportKeys is the file that moves the keys of userID to another browser,
// locked with code (newCode), as JSON.
export async function exportKeys(userID, code) {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(IV_SIZE));
  const plain = encoder.encode(JSON.stringify({ user: userID, keys: mine(userID).keys }));
  const data = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, await codeKey(code, salt), plain);

  return JSON.stringify({ freenet: 'message keys', version: 1, salt: toBase64(salt), iv: toBase64(iv), data: toBase64(new Uint8Array(data)) });
}

// importKeys takes the keys of userID from a file exportKeys made into
// this browser, with what it keeps already, and tells the state of the key
// here then. A wrong code or file throws a sentence to show.
export async function importKeys(userID, file, code) {
  let keys;

  try {
    const box = JSON.parse(file);
    const plain = await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: fromBase64(box.iv) },
      await codeKey(code, fromBase64(box.salt)),
      fromBase64(box.data),
    );
    const content = JSON.parse(decoder.decode(plain));

    if (content.user !== userID) throw 'Это ключ другой страницы.';
    keys = content.keys;
  } catch (e) {
    if (typeof e === 'string') throw e;
    throw 'Не удалось открыть файл: проверьте код и файл.';
  }

  const kept = mine(userID).keys;
  const known = new Set(kept.map((k) => k.public));

  keep(userID, { keys: [...kept, ...keys.filter((k) => !known.has(k.public))], state: '', checkedAt: 0 });

  return ensureKey(userID, { force: true });
}

// Derived AES keys, by the two public keys: each pair is derived once a
// page.
const derived = new Map();

function aesKey(own, peerPublic) {
  const id = own.public + ' ' + toBase64(peerPublic);
  if (!derived.has(id)) derived.set(id, derive(own, peerPublic));

  return derived.get(id);
}

async function derive(own, peerPublic) {
  const ownPublic = fromBase64(own.public);
  const [privateKey, publicKey] = await Promise.all([
    crypto.subtle.importKey('pkcs8', fromBase64(own.private), { name: 'X25519' }, false, ['deriveBits']),
    crypto.subtle.importKey('raw', peerPublic, { name: 'X25519' }, true, []),
  ]);
  const secret = await crypto.subtle.deriveBits({ name: 'X25519', public: publicKey }, privateKey, 256);
  const base = await crypto.subtle.importKey('raw', secret, 'HKDF', false, ['deriveKey']);
  const [lo, hi] = compareBytes(ownPublic, peerPublic) <= 0 ? [ownPublic, peerPublic] : [peerPublic, ownPublic];

  return crypto.subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(0), info: concat(encoder.encode(INFO), lo, hi) },
    base,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt'],
  );
}

// seal is what a message from userID to the owner of peerKey (base64)
// says, sealed, base64: payload is { t: its words, f: the words of the
// messages it carries on, in the order of all of them }.
export async function seal(userID, peerKey, payload) {
  const own = mine(userID).keys[0];
  if (!own) throw 'На этом устройстве нет ключа шифрования сообщений.';

  const peerPublic = fromBase64(peerKey);
  const iv = crypto.getRandomValues(new Uint8Array(IV_SIZE));
  const header = concat(new Uint8Array([VERSION]), fromBase64(own.public), peerPublic, iv);
  const sealed = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv, additionalData: header },
    await aesKey(own, peerPublic),
    encoder.encode(JSON.stringify(payload)),
  );

  return toBase64(concat(header, new Uint8Array(sealed)));
}

// open is what a sealed message of a dialog of userID says ({ t, f }), or
// null when none of the keys kept here opens it.
export async function open(userID, sealedText) {
  try {
    const sealed = fromBase64(sealedText);
    if (sealed.length < HEADER_SIZE || sealed[0] !== VERSION) return null;

    const sender = sealed.subarray(1, 1 + KEY_SIZE);
    const recipient = sealed.subarray(1 + KEY_SIZE, 1 + 2 * KEY_SIZE);
    const iv = sealed.subarray(1 + 2 * KEY_SIZE, HEADER_SIZE);

    for (const own of mine(userID).keys) {
      const ownPublic = fromBase64(own.public);
      let peerPublic = null;

      if (compareBytes(ownPublic, sender) === 0) peerPublic = recipient;
      else if (compareBytes(ownPublic, recipient) === 0) peerPublic = sender;
      if (!peerPublic) continue;

      const plain = await crypto.subtle.decrypt(
        { name: 'AES-GCM', iv, additionalData: sealed.subarray(0, HEADER_SIZE) },
        await aesKey(own, peerPublic.slice()),
        sealed.subarray(HEADER_SIZE),
      );
      const payload = JSON.parse(decoder.decode(plain));

      return { t: String(payload.t ?? ''), f: Array.isArray(payload.f) ? payload.f.map((x) => String(x ?? '')) : [] };
    }
  } catch {
    // A message that does not open says nothing readable.
  }

  return null;
}

// openMessages puts into messages of dialogs of userID what they say:
// text, and the text of each message carried on (by its index). A
// message none of the keys here opens is unreadable, saying nothing;
// words keeps what it said, for an edit to seal again.
export async function openMessages(userID, messages) {
  await Promise.all(messages.map(async (m) => {
    if (!m || m.text !== undefined) return;

    const said = m.sealed ? await open(userID, m.sealed) : { t: '', f: [] };

    m.unreadable = !said;
    m.text = said?.t ?? '';
    m.words = said?.f ?? [];
    for (const f of m.forwarded ?? []) f.text = m.words[f.index] ?? '';
  }));

  return messages;
}

// carriedWords are the words of messages to forward, in the order the
// server keeps them: oldest first, each followed by what it carries on.
// A message that does not open cannot be carried on; one of a chat whose
// text the page has only the beginning of is read whole first.
export async function carriedWords(messages) {
  const whole = await Promise.all([...messages].sort((a, b) => (a.id < b.id ? -1 : 1))
    .map((m) => (m.textCut && m.chatId ? api.chatMessage(m.id) : m)));

  return whole.flatMap((m) => {
    if (m.unreadable) throw 'Сообщение не расшифровано, его нельзя переслать.';

    const under = Array.from({ length: m.forwardedCount ?? 0 }, () => '');
    // In a dialog, what it carries says its words sealed with it; in a
    // chat, each shown says its own (those not shown are gone anyway).
    if (m.words) m.words.forEach((w, i) => { if (i < under.length) under[i] = w; });
    else for (const f of m.forwarded ?? []) under[f.index] = f.text ?? '';

    return [m.text ?? '', ...under];
  });
}

// NO_PEER_KEY is why one cannot write to a person who has no key yet.
export const NO_PEER_KEY = 'У собеседника ещё нет ключа шифрования сообщений: написать можно будет, когда собеседник зайдёт на сайт.';

// peerKey is the key messages to nickname are sealed with; it throws a
// sentence when they have none yet.
async function peerKey(nickname) {
  const { publicKey } = await api.messageKey(nickname);
  if (!publicKey) throw NO_PEER_KEY;

  return publicKey;
}

// ready throws a sentence when this browser cannot seal for userID.
async function ready(userID, force = false) {
  const state = await ensureKey(userID, { force });
  if (state !== KEY_READY) throw 'На этом устройстве нет ключа шифрования сообщений: перенесите его с другого устройства в «Настройках».';
}

// withKeys runs write with what the message says sealed for the keys of
// both sides as the server has them; when one of them changed meanwhile
// (409), once more with the new ones.
async function withKeys(userID, nickname, payload, write) {
  for (let attempt = 0; ; attempt++) {
    await ready(userID, attempt > 0);

    const says = payload.t !== '' || payload.f.some(Boolean);
    const sealed = says ? await seal(userID, await peerKey(nickname), payload) : null;

    try {
      return await write(sealed);
    } catch (e) {
      if (attempt > 0 || e?.status !== 409 || e.message !== 'the encryption keys changed') throw e;
    }
  }
}

// sendSealed writes text from userID to nickname, with forward carried on
// (their words sealed with it) and photos, a post or a photo of an album.
export async function sendSealed(userID, nickname, text, { forward = [], photos = [], post = '', albumPhoto = '' } = {}) {
  const payload = { t: text, f: await carriedWords(forward) };

  const m = await withKeys(userID, nickname, payload, (sealed) =>
    api.sendMessage(nickname, { sealed, photos, post, albumPhoto, forward: forward.map((f) => f.id) }));

  return (await openMessages(userID, [m]))[0];
}

// editSealed makes m, a message of userID to nickname, say text: what it
// carries on keeps its words. The edit names what m said when it began,
// so one made elsewhere meanwhile is not overwritten.
export async function editSealed(userID, nickname, m, text, { add = [], remove = [] } = {}) {
  const payload = { t: text, f: m.words ?? [] };

  const edited = await withKeys(userID, nickname, payload, (sealed) =>
    api.editMessage(m.id, { sealed, base: m.sealed ?? '', add, remove }));

  return (await openMessages(userID, [edited]))[0];
}

// safetyCode is what two people compare to be sure their messages are
// sealed for each other and no one between: 20 digits from both keys.
export async function safetyCode(keyA, keyB) {
  const [a, b] = [fromBase64(keyA), fromBase64(keyB)];
  const [lo, hi] = compareBytes(a, b) <= 0 ? [a, b] : [b, a];
  const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', concat(lo, hi)));
  const groups = [];

  for (let i = 0; i < 10; i += 2) groups.push(String(((hash[i] << 8) | hash[i + 1]) % 10000).padStart(4, '0'));

  return groups.join(' ');
}

// notePeerKey remembers the key of a peer as first seen and tells whether
// it is the same now: 'new', 'same' or 'changed'. A changed key is
// remembered from now on.
export function notePeerKey(userID, peerID, key) {
  const all = read(PEERS);
  const known = all[userID]?.[peerID];

  if (known === key) return 'same';

  all[userID] = { ...all[userID], [peerID]: key };
  write(PEERS, all);

  return known ? 'changed' : 'new';
}

// ownKey is the public key this browser seals with for userID; '' for
// none.
export function ownKey(userID) {
  return mine(userID).keys[0]?.public ?? '';
}
