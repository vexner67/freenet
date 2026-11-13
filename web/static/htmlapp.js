// HTML documents attached to posts and messages run where they are shown,
// a game say: in a frame of the sandbox page (/sandbox of the gateway),
// whose scripts have an origin of their own, no network and nothing of the
// site. A page runs only when asked ("Запустить"), so a feed of them stays
// light, and in a box of a fixed size, so nothing moves when it starts.
//
// A page may ask the site to remember it: window.freenet.save(data),
// freenet.load() and freenet.score(points) of the sandbox page send their
// requests here, and this page answers them as the signed-in player. The
// page never learns who plays, sees no token and reaches no network: it
// only asks, and only for itself, the document it is.

import { api, signedIn } from './api.js';
import { el, fullName, pagePath } from './layout.js';
import { groupedVotes } from './labels.js';
import { tr } from './i18n.js';

// SANDBOX is the page the frame opens; READY is what it says when it waits
// for the page to show.
const SANDBOX = '/sandbox';
const READY = 'freenet-sandbox-ready';

// SEND_EVERY is how often a game's saves and scores reach the server at
// most: a game may save on every move, and only the last save matters.
const SEND_EVERY = 5000;
// MAX_SAVE is how large a save is, as the post service says.
const MAX_SAVE = 64 * 1024;

// isHTMLDocument tells the documents that run as pages.
export const isHTMLDocument = (doc) => doc?.ext === 'html';

// htmlNode is an HTML document as a box: its screen, where it runs, with
// "Запустить" until it does, and under it its name, which downloads it,
// "Во весь экран" and "Запустить" or "Остановить".
export function htmlNode(doc) {
  const screen = el('div', { className: 'html-app-screen' });
  const toggle = el('a', { className: 'html-app-toggle' });
  const full = el('a', { className: 'html-app-toggle', textContent: tr('Во весь экран') });
  const error = el('span', { className: 'html-app-error', hidden: true });
  const records = el('div', { className: 'html-app-records', hidden: true });
  const games = gameBridge(doc, () => showRecords(doc, records));
  let stop = null;
  let frame = null;

  function idle() {
    if (stop) games.flush();
    stop?.();
    stop = null;
    frame = null;
    screen.replaceChildren(el('a', { className: 'button', textContent: tr('Запустить'), onclick: run }));
    toggle.textContent = tr('Запустить');
  }

  async function run() {
    error.hidden = true;
    toggle.textContent = tr('Остановить');
    let html;
    try {
      const res = await fetch(doc.url);
      if (!res.ok) throw new Error(String(res.status));
      html = await res.text();
    } catch {
      idle();
      error.textContent = tr('Не удалось загрузить страницу.');
      error.hidden = false;
      return;
    }
    ({ stop, frame } = start(screen, html, doc.name, games.ask));
    showRecords(doc, records);
  }

  const box = el('div', { className: 'html-app' },
    screen,
    el('div', { className: 'html-app-info' },
      el('span', { className: 'doc-icon', textContent: doc.ext }),
      el('a', { className: 'doc-name', href: doc.url, download: doc.name, textContent: doc.name }),
      error,
      full,
      toggle),
    records,
  );

  // "Во весь экран" gives the box the whole screen: the browser's full
  // screen where there is one, and the window otherwise (an iPhone has it
  // for videos only). The frame stays the same sandbox; only its box grows.
  const isFull = () => box.classList.contains('html-app-full');
  function shrink() {
    box.classList.remove('html-app-full');
    full.textContent = tr('Во весь экран');
    if (document.fullscreenElement === box) document.exitFullscreen().catch(() => {});
  }
  // The full screen is asked for first, while the click still allows it.
  function grow() {
    box.classList.add('html-app-full');
    full.textContent = tr('Свернуть');
    box.requestFullscreen?.().catch(() => {});
    if (stop) frame?.focus();
    else run();
  }
  full.addEventListener('click', () => (isFull() ? shrink() : grow()));
  // Esc of the browser leaves its full screen; the box follows.
  box.addEventListener('fullscreenchange', () => { if (!document.fullscreenElement && isFull()) shrink(); });

  toggle.addEventListener('click', () => {
    if (!stop) {
      run();
      return;
    }
    if (isFull()) shrink();
    idle();
  });
  idle();

  return box;
}

// start shows html in a sandboxed frame in screen and returns the frame
// and what stops it. The frame asks for the page when it is ready; only its
// own frame is answered. The page gets the keyboard at once: a game led by
// the arrows should not need a click first.
function start(screen, html, name, ask) {
  const frame = el('iframe', {
    className: 'html-app-frame',
    sandbox: 'allow-scripts allow-pointer-lock',
    referrerPolicy: 'no-referrer',
    title: name,
  });
  // Set once: el() sets an address twice, and a frame given its address
  // again loads anew, losing the page sent to the first load.
  frame.src = SANDBOX;

  let shown = false;
  function onMessage(event) {
    if (event.source !== frame.contentWindow) return;
    if (!shown) {
      if (event.data !== READY) return;
      shown = true;
      // The frame has no origin of its own to name.
      frame.contentWindow.postMessage(html, '*');
      frame.focus();
      return;
    }
    answer(frame, event.data, ask);
  }

  addEventListener('message', onMessage);
  screen.replaceChildren(frame);

  return {
    frame,
    stop: () => {
      removeEventListener('message', onMessage);
      frame.remove();
    },
  };
}

// answer replies to a request of the page in frame: { freenet: 'save' |
// 'load' | 'score', id, value }, anything else is not looked at.
function answer(frame, request, ask) {
  if (!request || typeof request !== 'object' || !Number.isSafeInteger(request.id)) return;
  if (!['save', 'load', 'score'].includes(request.freenet)) return;

  const reply = (fields) => frame.contentWindow?.postMessage({ freenet: 'reply', id: request.id, ...fields }, '*');
  ask(request.freenet, request.value).then(
    (value) => reply({ value: value ?? null }),
    (err) => reply({ error: String(err?.message || err) }),
  );
}

// gameBridge is what the site does for the page of doc: ask answers its
// requests, flush sends what waits when the page stops, and scored is
// told of every score kept.
function gameBridge(doc, scored) {
  const saves = throttled((data) => api.saveGame(doc.id, data));
  const scores = throttled(async (score) => {
    const { best } = await api.gameScore(doc.id, score);
    scored();
    return best;
  });

  async function ask(kind, value) {
    if (kind === 'load') {
      if (!signedIn()) return null;
      const save = await api.gameSave(doc.id);
      return save.found ? save.data : null;
    }

    // A guest plays, but nothing of theirs is kept.
    if (!signedIn()) throw new Error('sign in to save');

    if (kind === 'save') {
      if (typeof value !== 'string') throw new Error('nothing to save');
      if (new TextEncoder().encode(value).length > MAX_SAVE) throw new Error('a save is at most 64 KB');
      await saves.push(value);
      return null;
    }

    if (!Number.isSafeInteger(value) || value < 0) throw new Error('a score is a whole number from 0');
    return scores.push(value, Math.max);
  }

  return {
    ask,
    flush: () => {
      saves.flush();
      scores.flush();
    },
  };
}

// throttled sends what it is given at most once every SEND_EVERY: what
// comes in between is merged into one (the last by default) and sent
// after, and every caller learns how that went.
function throttled(send) {
  let pending = null;
  let busy = false;
  let sent = 0;
  let timer = 0;

  async function flush() {
    clearTimeout(timer);
    timer = 0;
    if (!pending || busy) return;

    const { value, waiters } = pending;
    pending = null;
    busy = true;
    sent = Date.now();
    try {
      const result = await send(value);
      for (const w of waiters) w.resolve(result);
    } catch (err) {
      for (const w of waiters) w.reject(err);
    } finally {
      busy = false;
      if (pending) schedule();
    }
  }

  function schedule() {
    if (!timer) timer = setTimeout(flush, Math.max(0, sent + SEND_EVERY - Date.now()));
  }

  return {
    push(value, merge = (_, next) => next) {
      return new Promise((resolve, reject) => {
        pending = pending
          ? { value: merge(pending.value, value), waiters: [...pending.waiters, { resolve, reject }] }
          : { value, waiters: [{ resolve, reject }] };
        if (!busy) schedule();
      });
    },
    flush,
  };
}

// showRecords fills "Рекорды" under the page of doc: the best results of
// the friends of the signed-in person and theirs, lines "имя — очки". It
// stays hidden while there are none, and for a guest.
async function showRecords(doc, box) {
  if (!signedIn()) return;

  let found;
  try {
    ({ records: found } = await api.gameRecords(doc.id));
  } catch {
    return;
  }

  box.replaceChildren(
    el('div', { className: 'html-app-records-title', textContent: tr('Рекорды') }),
    ...found.map((r) => el('div', { className: 'html-app-record' },
      el('a', { href: pagePath(r.user), textContent: fullName(r.user) }),
      ' — ' + groupedVotes(r.score))),
  );
  box.hidden = found.length === 0;
}
