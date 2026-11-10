// The live events of the page (WebSocket /api/v1/events): new messages and
// read marks of your dialogs, changes of your notifications and both menu
// counters. One connection per page, opened by renderChrome for a signed-in
// user. It connects again by itself: with a fresh token when the old one
// expired (close code 4001), after a pause when it broke. While it is down
// pages fall back to what they did before, polling (see isLive).

import { renewSession, session } from './api.js';

const TOKEN_EXPIRED = 4001;
// Pauses before connecting again, longer after each failure.
const RETRY_MS = [1000, 2000, 5000, 10000, 30000];

const handlers = new Map();
let live = false;
let started = false;
// paused is a page left behind the frame of the player (shell.js): it
// keeps still, and the page in the frame has the events.
let paused = false;
let socket = null;
let reopen = () => {};
let roomUsers = 0;

export function retainRoomEvents() {
  roomUsers++;
  if (paused && started && !socket) reopen();
  return () => { roomUsers = Math.max(0, roomUsers - 1); if (paused && !roomUsers) socket?.close(1000); };
}

// onEvent calls fn with every event of type: "ready" (connected, with
// both counters), "counters", "message", "read", "notification".
export function onEvent(type, fn) {
  if (!handlers.has(type)) handlers.set(type, new Set());
  handlers.get(type).add(fn);
}

// isLive tells whether events are coming: pages poll only when not.
export function isLive() {
  return live || paused;
}

// pauseEvents closes the connection of a page that stays behind the
// frame of the player, so that a message or a call is not heard twice;
// resumeEvents opens it again when the page comes back.
export function pauseEvents() {
  paused = true;
  if (!roomUsers) socket?.close(1000);
}

export function resumeEvents() {
  if (!paused) return;
  paused = false;
  if (started && !socket) reopen();
}

function dispatch(event) {
  if (paused && roomUsers && !event.type.startsWith('room_') && event.type !== 'ready') return;
  for (const fn of handlers.get(event.type) ?? []) {
    try {
      fn(event);
    } catch (e) {
      console.error(e);
    }
  }
}

export function connectEvents() {
  if (started || typeof WebSocket === 'undefined') return;
  started = true;

  let failures = 0;

  function retry() {
    setTimeout(open, RETRY_MS[Math.min(failures++, RETRY_MS.length - 1)]);
  }

  function open() {
    const tokens = session.get();
    if (!tokens || (paused && !roomUsers)) return; // signed out, or left behind

    const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${scheme}://${location.host}/api/v1/events`);
    socket = ws;

    ws.addEventListener('open', () => ws.send(JSON.stringify({ token: tokens.accessToken })));

    ws.addEventListener('message', (message) => {
      const event = JSON.parse(message.data);
      if (event.type === 'ready') {
        failures = 0;
        live = true;
      }
      dispatch(event);
    });

    ws.addEventListener('close', async (event) => {
      live = false;
      if (socket === ws) socket = null;
      if (paused && !roomUsers) return;

      if (event.code !== TOKEN_EXPIRED) {
        retry();
        return;
      }

      try {
        await renewSession(tokens.accessToken);
      } catch (e) {
        if (e.status === 401) return; // the session is over
        retry();
        return;
      }

      // Refreshed at once, unless that keeps failing.
      if (failures++ === 0) open(); else retry();
    });
  }

  reopen = open;
  open();
}
