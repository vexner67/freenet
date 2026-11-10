// Music that plays on from page to page, as on openvk: while a song plays,
// a page of the site opened from this one opens in a frame over it, and
// the player stays here, out of the frame. The tab is the shell — its
// address and title follow the page in the frame, back and forward go
// through the pages of the frame, and back past the first of them shows
// the page the song started on, which waited under the frame. The pages
// in the frame play their songs here (audio.js, framedPlayer).
//
// What a page opens in the frame: a link, a form that asks (GET) and an
// address set by a script — any page of this site but the API, in this
// tab. A new tab, a download, a form that sends and the address bar of
// the browser go as they did.

import { pauseEvents, resumeEvents } from './events.js';

const root = document.documentElement;
const SHELL = 'freenetShell';

let frame = null;
let watching = null;
let playing = () => false;
let framing = () => {};
const reasons = new Set();
const frameListeners = new Set();
let installed = false;

// startShell makes the tab the shell of the pages while playing() says a
// song is in the player; onFrame hears the frame come (true) and go.
export function startShell(isPlaying, onFrame = () => {}) {
  reasons.add(isPlaying);
  frameListeners.add(onFrame);
  playing = () => [...reasons].some((reason) => reason());
  framing = (open) => { for (const listener of frameListeners) listener(open); };
  if (installed) return;
  installed = true;

  if (window.navigation?.addEventListener) {
    window.navigation.addEventListener('navigate', onNavigate);
  } else {
    document.addEventListener('click', onClick);
    document.addEventListener('submit', onSubmit);
  }
  window.addEventListener('popstate', onPopState);
}

// tellFrame sends the page in the frame a message of the player.
export function tellFrame(message) {
  frame?.contentWindow?.postMessage(message, location.origin);
}

// opens tells whether going to url keeps the song: a page of the site.
function opens(url) {
  return url.origin === location.origin && !url.pathname.startsWith('/api/') && !url.pathname.startsWith('/static/');
}

function onNavigate(event) {
  if (!playing() || !event.cancelable) return;
  if (event.navigationType !== 'push' && event.navigationType !== 'replace') return;
  if (event.destination.sameDocument || event.hashChange || event.downloadRequest !== null || event.formData) return;

  const url = new URL(event.destination.url);
  if (!opens(url)) return;

  event.preventDefault();
  enter(url.href, true);
}

function onClick(event) {
  if (!playing() || event.defaultPrevented || event.button !== 0) return;
  if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;

  const link = event.target.closest?.('a[href]');
  if (!link || (link.target && link.target !== '_self') || link.hasAttribute('download')) return;

  const url = new URL(link.href, location.href);
  if (!opens(url) || (url.pathname === location.pathname && url.search === location.search && url.hash)) return;

  event.preventDefault();
  enter(url.href, true);
}

function onSubmit(event) {
  const form = event.target;
  if (!playing() || event.defaultPrevented || form.method !== 'get' || form.target) return;

  const url = new URL(form.action, location.href);
  if (!opens(url)) return;

  url.search = new URLSearchParams(new FormData(form)).toString();
  event.preventDefault();
  enter(url.href, true);
}

// enter opens href in the frame; push adds the page to the history of
// the tab (not when going forward to it again).
function enter(href, push) {
  // Links in the persistent room bar belong to the outer document too.
  // Route them into the existing frame instead of reloading its connections.
  if (frame) {
    frame.src = href;
    if (push) history.pushState({ [SHELL]: true }, '', href);
    framing(true);
    return;
  }
  frame = document.createElement('iframe');
  frame.className = 'shell-frame';
  frame.title = document.title;
  frame.src = href;
  frame.addEventListener('load', follow);
  document.body.append(frame);
  root.classList.add('shell');
  if (push) history.pushState({ [SHELL]: true }, '', href);
  pauseEvents();
  framing(true);
}

// leave shows the page the song started on again.
function leave() {
  watching?.disconnect();
  watching = null;
  frame?.remove();
  frame = null;
  root.classList.remove('shell');
  document.title = root.dataset.title ?? document.title;
  resumeEvents();
  framing(false);
}

// follow gives the tab the address and the title of the page in the frame.
function follow() {
  let doc;
  let href;
  try {
    doc = frame.contentDocument;
    href = frame.contentWindow.location.href;
  } catch {
    return; // a page of another site, which keeps its address to itself
  }
  if (!doc || href === 'about:blank') return;

  root.dataset.title ??= document.title;
  if (href !== location.href) history.replaceState({ [SHELL]: true }, '', href);

  const title = () => {
    document.title = doc.title;
    frame.title = doc.title;
  };
  title();
  watching?.disconnect();
  watching = new MutationObserver(title);
  watching.observe(doc.head ?? doc.documentElement, { childList: true, subtree: true, characterData: true });
  frame.contentWindow.focus();
}

function onPopState(event) {
  const shell = Boolean(event.state?.[SHELL]);
  if (shell && !frame) enter(location.href, false);
  else if (!shell && frame) leave();
}
