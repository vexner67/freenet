// One room connection belongs to the outer document, alongside the music
// shell. Frames receive snapshots and send commands, never media tokens.
import { api, session } from './api.js';
import { el } from './layout.js';
import { errorText } from './labels.js';
import { onEvent, retainRoomEvents } from './events.js';
import { startShell } from './shell.js';
import { playbackPosition, roomClockOffset, roomPlaybackLeader } from './room-sync.js';
import { tr } from './i18n.js';

const STORE = 'freenet.room';

export async function roomRuntime() {
  try {
    if (window.parent !== window && window.parent.freenetEnsureRoom) return window.parent.freenetEnsureRoom();
  } catch { /* A room is never shared with another origin. */ }
  if (!window.freenetRoom) window.freenetRoom = createRuntime();
  return window.freenetRoom;
}

function createRuntime() {
  const tab = crypto.randomUUID();
  let room = null;
  let meID = '';
  let joined = false;
  let joining = false;
  let voice = null;
  let screen = null;
  let muted = true;
  let speaking = [];
  let status = '';
  let error = '';
  let chat = [];
  let blockedIDs = [];
  let offset = 0;
  let generation = 0;
  let heartbeat = null;
  let releaseEvents = null;
  let commands = Promise.resolve();
  let rejoining = null;
  let reconnecting = false;
  let reconnectTimer = null;
  const listeners = new Set();
  const audio = el('div', { className: 'room-voice-audio' });
  const title = el('a', { className: 'room-banner-title' });
  const count = el('span');
  const mic = el('button', { type: 'button', className: 'button' });
  const leaveButton = el('button', { type: 'button', className: 'button', textContent: tr('Выйти') });
  const back = el('a', { textContent: tr('Вернуться') });
  const banner = el('div', { className: 'room-banner', hidden: true }, title, count, mic, back, leaveButton);
  document.body.append(audio, banner);
  const snapshot = () => JSON.stringify({ room, meID, joined, joining, muted, speaking, status, error, chat });
  const emit = () => {
    banner.hidden = !joined;
    document.documentElement.classList.toggle('with-room', joined);
    title.textContent = room ? tr('Комната: {0}', room.title) : '';
    title.href = room ? `/room/${encodeURIComponent(room.id)}` : '/rooms';
    back.href = title.href;
    count.textContent = room ? tr('{0} из {1}', room.members.length, room.maxMembers) : '';
    mic.textContent = muted ? tr('Включить микрофон') : tr('Выключить микрофон');
    for (const fn of listeners) fn(snapshot());
  };
  const report = (e) => { error = typeof e === 'string' ? e : errorText(e); emit(); };
  const keep = () => {
    if (!joined || !room) return;
    localStorage.setItem(STORE, JSON.stringify({ tab, room: room.id, account: meID, until: Date.now() + 45_000 }));
  };
  const beacon = () => {
    const token = session.get()?.accessToken;
    if (joined && room && token) navigator.sendBeacon(`/api/v1/rooms/${encodeURIComponent(room.id)}/leave`, new Blob([JSON.stringify({ token })], { type: 'application/json' }));
  };
  startShell(() => joined, (open) => { screen?.dock(null); if (!open) emit(); });

  async function dispose(send = true) {
    const previous = room;
    joined = false;
    joining = false;
    generation++;
    clearTimeout(reconnectTimer);
    clearInterval(heartbeat);
    heartbeat = null;
    releaseEvents?.();
    releaseEvents = null;
    if (voice) { const old = voice; voice = null; await old.leave(); }
    screen?.close();
    screen = null;
    speaking = [];
    muted = true;
    try {
      if (JSON.parse(localStorage.getItem(STORE) ?? 'null')?.tab === tab) localStorage.removeItem(STORE);
    } catch { /* A private browser may keep no tab state. */ }
    emit();
    if (send && previous) await api.leaveRoom(previous.id);
  }

  async function connect(media, mutedIDs, members, expected) {
    const { joinVoice } = await import('/static/room-voice.js');
    const made = await joinVoice(media, { root: audio, mutedIDs, memberIDs: members.map((m) => m.user.id),
      onSpeaking: (ids) => { speaking = ids; emit(); },
      onMuted: (value) => { muted = value; emit(); },
      onState: (value) => {
        if (expected !== generation) return;
        status = value; emit();
        if (value === 'disconnected' && joined) {
          clearTimeout(reconnectTimer);
          reconnectTimer = setTimeout(() => reconnect().catch(report), 2000);
        }
      },
    });
    if (expected !== generation || !joined) { await made.leave(); return; }
    voice = made;
  }

  async function reconnect() {
    if (!joined || joining || reconnecting || !room) return;
    reconnecting = true;
    const expected = generation;
    const id = room.id;
    try {
      // A fresh media token is issued only after checking membership and access.
      const media = await api.roomToken(id);
      if (expected !== generation || !joined) return;
      const old = voice;
      voice = null;
      await old?.leave();
      await connect(media.media, blockedIDs, room.members, expected);
      await refresh();
    } catch (e) {
      if (expected !== generation) return;
      if (e.status === 403 || e.status === 404 || e.status === 409) await dispose(false);
      else reconnectTimer = setTimeout(() => reconnect().catch(report), 5000);
      throw e;
    } finally { reconnecting = false; }
  }

  async function refresh() {
    if (!joined || joining || !room) return;
    if (rejoining) return rejoining;
    const id = room.id;
    const expected = generation;
    rejoining = (async () => {
      try {
        const sent = Date.now();
        const next = await api.joinRoom(id);
        if (expected !== generation || !joined) return;
        room = next.room;
        chat = next.chat;
        blockedIDs = next.mutedIds;
        offset = roomClockOffset(next.serverTime, sent, Date.now());
        voice?.members(next.mutedIds, next.room.members.map((m) => m.user.id));
        await screen?.apply(room.playback, offset);
        emit();
      } catch (e) {
        if (e.status === 403 || e.status === 404 || e.status === 409) await dispose(false);
        report(e);
      } finally { rejoining = null; }
    })();
    return rejoining;
  }

  async function join(id, userID) {
    if (joining) return;
    if (window.freenetCallBusy?.()) throw new Error('end the call before joining a room');
    if (joined && room?.id === id) { await voice?.startAudio(); return; }
    joining = true;
    const expected = ++generation;
    error = '';
    emit();
    try {
      const sent = Date.now();
      const result = await api.joinRoom(id);
      if (expected !== generation) return;
      const oldVoice = voice;
      voice = null;
      if (oldVoice) await oldVoice.leave();
      if (expected !== generation) return;
      screen?.close();
      room = result.room;
      chat = result.chat;
      blockedIDs = result.mutedIds;
      meID = userID;
      joined = true;
      offset = roomClockOffset(result.serverTime, sent, Date.now());
      releaseEvents ??= retainRoomEvents();
      const { createRoomScreen } = await import('/static/room-screen.js');
      if (expected !== generation) return;
      screen = createRoomScreen({ onError: report, onEnded: (version) => {
        if (room && roomPlaybackLeader(room.members.map((member) => member.user.id), room.ownerId, room.control) === meID) command('next', undefined, version).catch(report);
      } });
      await screen.apply(room.playback, offset);
      if (expected !== generation || !joined) return;
      clearInterval(heartbeat);
      heartbeat = setInterval(async () => {
        if (!joined || document.visibilityState === 'unloaded') return;
        try { await api.roomHeartbeat(room.id); keep(); await refresh(); }
        catch (e) {
          if (e.status === 403 || e.status === 404 || e.status === 409) await dispose(false);
          report(e);
        }
      }, 20_000);
      keep();
      emit();
      await connect(result.media, result.mutedIds, room.members, expected);
      if (expected !== generation || !joined) return;

    } catch (e) {
      if (expected !== generation) return;
      if (joined && room?.id === id) await dispose(true).catch(report);
      report(e);
      throw e;
    } finally { if (expected === generation) joining = false; emit(); }
  }

  async function change(action, value, expectedVersion) {
    if (!joined || !room) return;
    const p = room.playback;
    if (expectedVersion !== undefined && p.version !== expectedVersion) return;
    const next = { kind: p.kind, mediaId: p.mediaId, playing: p.playing,
      position: playbackPosition(p, Date.now() + offset), version: p.version, queue: [...p.queue] };
    if (action === 'toggle') next.playing = !next.playing;
    if (action === 'seek') next.position = Math.max(0, Number(value) || 0);
    if (action === 'load') Object.assign(next, { kind: value.kind, mediaId: value.mediaId, playing: true, position: 0 });
    if (action === 'enqueue') {
      if (!p.kind) Object.assign(next, { kind: value.kind, mediaId: value.mediaId, playing: true, position: 0 });
      else {
        if (next.queue.length >= 50) throw new Error('room queue is full');
        next.queue.push(value);
      }
    }
    if (action === 'next') {
      const first = next.queue.shift();
      if (first) Object.assign(next, { kind: first.kind, mediaId: first.mediaId, playing: true, position: 0 });
      else next.playing = false;
    }
    try {
      const result = await api.roomPlayback(room.id, next);
      room.playback = result.playback;
      if (next.kind) room.kind = next.kind === 'youtube' ? 'watch' : 'listen';
      await screen?.apply(room.playback, offset);
      emit();
    } catch (e) { if (e.status === 409) await refresh(); throw e; }
  }
  function command(action, value, expectedVersion) {
    commands = commands.catch(() => {}).then(() => change(action, value, expectedVersion));
    return commands;
  }

  for (const kind of ['room_join', 'room_leave', 'room_update', 'room_closed', 'room_kick', 'room_chat', 'room_playback']) {
    onEvent(kind, (event) => {
      if (!joined || event.roomId !== room?.id) return;
      if (kind === 'room_kick' || kind === 'room_closed' || (kind === 'room_leave' && event.userId === meID && !joining)) {
        dispose(false).then(() => report(kind === 'room_kick' ? tr('Вас удалили из комнаты.') : tr('Вы больше не в этой комнате.'))).catch(report);
      } else if (kind === 'room_playback' && event.playback.version > room.playback.version) {
        room.playback = event.playback;
        if (event.playback.kind) room.kind = event.playback.kind === 'youtube' ? 'watch' : 'listen';
        screen?.apply(room.playback, offset).catch(report);
        emit();
      } else if (kind === 'room_chat' && event.message) {
        if (!chat.some((m) => m.id === event.message.id)) chat = [...chat, event.message].slice(-50);
        emit();
      } else if (kind !== 'room_chat') refresh().catch(report);
    });
  }
  onEvent('ready', () => refresh().catch(report));
  mic.addEventListener('click', () => mute(!muted).catch(report));
  leaveButton.addEventListener('click', () => dispose().catch(report));
  window.addEventListener('pagehide', () => { beacon(); dispose(false).catch(() => {}); });
  window.addEventListener('storage', (event) => {
    if (!joined && !joining) return;
    if (event.key === 'freenet.account') { dispose(false).catch(report); return; }
    if (event.key === STORE && event.newValue) {
      try { const next = JSON.parse(event.newValue); if (next.tab !== tab && next.account === meID) dispose(false).catch(report); }
      catch { /* A malformed tab flag grants nobody entry. */ }
    }
  });
  async function mute(value) { await voice?.mute(value); muted = value; emit(); }
  emit();
  return {
    snapshot, join, active: () => joined, leave: () => dispose(), mute, command,
    async sendChat(text = '', sticker = '') {
      if (!joined || !room) return;
      const result = await api.roomChat(room.id, text, sticker);
      if (!chat.some((message) => message.id === result.message.id)) chat = [...chat, result.message].slice(-50);
      emit();
    },
    onChange(fn) { listeners.add(fn); fn(snapshot()); return () => listeners.delete(fn); },
    silence(id, value) { voice?.silence(id, value); },
    blocked(id) { return voice?.blocked(id) ?? false; },
    startAudio() { return voice?.startAudio(); },
    dockScreen(rect, framed) { screen?.dock(rect, framed); },
  };
}
