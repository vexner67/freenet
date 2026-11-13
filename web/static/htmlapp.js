// HTML documents attached to posts and messages run where they are shown,
// a game say: in a frame of the sandbox page (/sandbox of the gateway),
// whose scripts have an origin of their own, no network and nothing of the
// site. A page runs only when asked ("Запустить"), so a feed of them stays
// light, and in a box of a fixed size, so nothing moves when it starts.

import { el } from './layout.js';
import { tr } from './i18n.js';

// SANDBOX is the page the frame opens; READY is what it says when it waits
// for the page to show.
const SANDBOX = '/sandbox';
const READY = 'freenet-sandbox-ready';

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
  let stop = null;
  let frame = null;

  function idle() {
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
    ({ stop, frame } = start(screen, html, doc.name));
  }

  const box = el('div', { className: 'html-app' },
    screen,
    el('div', { className: 'html-app-info' },
      el('span', { className: 'doc-icon', textContent: doc.ext }),
      el('a', { className: 'doc-name', href: doc.url, download: doc.name, textContent: doc.name }),
      error,
      full,
      toggle),
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
function start(screen, html, name) {
  const frame = el('iframe', {
    className: 'html-app-frame',
    sandbox: 'allow-scripts allow-pointer-lock',
    referrerPolicy: 'no-referrer',
    title: name,
  });
  // Set once: el() sets an address twice, and a frame given its address
  // again loads anew, losing the page sent to the first load.
  frame.src = SANDBOX;

  function onMessage(event) {
    if (event.source !== frame.contentWindow || event.data !== READY) return;
    removeEventListener('message', onMessage);
    // The frame has no origin of its own to name.
    frame.contentWindow.postMessage(html, '*');
    frame.focus();
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
