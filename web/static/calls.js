// Calls between two people: the voice and the picture go straight between
// their browsers over WebRTC, encrypted by it (DTLS-SRTP). What sets a call
// up — rings, session descriptions with the fingerprints of those keys,
// network addresses — is sealed with the keys of the messages (e2ee.js), so
// the server only hands it on and cannot put itself between them. The code
// in the window comes from both fingerprints: the same code on both sides
// means nobody is in the middle. A TURN server, when the browsers cannot
// reach each other, carries packets it cannot read either.
//
// A signal is { k, id, tab, from, ... }: "ring", "cancel", "accept",
// "decline", "busy", "sdp", "ice", "camera", "end". A person's every page
// hears the signals of their calls, so the others stop ringing when one
// answers. The one who was called is the polite side of perfect negotiation.

import { api } from './api.js';
import { ensureKey, KEY_READY, NO_PEER_KEY, open as unseal, seal, sendSealed } from './e2ee.js';
import { onEvent } from './events.js';
import { tr } from './i18n.js';
import { avatarURL, el, fullName } from './layout.js';

const RING_FOR = 45_000;
const GONE_FOR = 20_000;
const SAY_FOR = 2_000;
const AUDIO_BITRATE = 64_000;
const VIDEO_BITRATE = 1_500_000;

const voice = { echoCancellation: true, noiseSuppression: true, autoGainControl: true };
const picture = { width: { ideal: 1280 }, height: { ideal: 720 }, frameRate: { ideal: 30 } };
const tab = crypto.randomUUID();

let me = null;
let call = null;

function timeOf(seconds) {
  const part = (n) => String(n).padStart(2, '0');
  const hours = Math.floor(seconds / 3600);

  return `${hours ? `${hours}:` : ''}${part(Math.floor(seconds / 60) % 60)}:${part(seconds % 60)}`;
}

// A two-tone ring drawn by the browser: no sound file to fetch. A page the
// person has not touched may not play it; the window and the title do.
function ringer() {
  const AudioContext = window.AudioContext ?? window.webkitAudioContext;
  if (!AudioContext) return () => {};

  const audio = new AudioContext();
  const beep = () => {
    for (const [at, pitch] of [[0, 620], [0.45, 520], [0.9, 620], [1.35, 520]]) {
      const tone = audio.createOscillator();
      const gain = audio.createGain();
      tone.frequency.value = pitch;
      gain.gain.setValueAtTime(0.0001, audio.currentTime + at);
      gain.gain.exponentialRampToValueAtTime(0.12, audio.currentTime + at + 0.03);
      gain.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + at + 0.4);
      tone.connect(gain).connect(audio.destination);
      tone.start(audio.currentTime + at);
      tone.stop(audio.currentTime + at + 0.42);
    }
    navigator.vibrate?.([400, 200, 400]);
  };
  beep();
  const timer = setInterval(beep, 3500);

  return () => {
    clearInterval(timer);
    audio.close().catch(() => {});
  };
}

function iconButton(icon, label, onclick, className = '') {
  const image = el('img', { src: icon, alt: '', width: 32, height: 32 });
  const caption = el('span', { textContent: label });
  const node = el('button', { type: 'button', className: `call-button ${className}`, onclick }, image, caption);

  return { node, image, caption };
}

// The window: a title bar with a lock, the person, how the call goes and a
// row of big buttons.
function drawWindow(peer) {
  const ui = {
    status: el('div', { className: 'call-status' }),
    timer: el('div', { className: 'call-timer', hidden: true }),
    bars: el('span', { className: 'call-bars', hidden: true }, ...[1, 2, 3, 4].map(() => el('i'))),
    code: el('div', { className: 'call-code', hidden: true }),
    buttons: el('div', { className: 'call-buttons' }),
    remote: el('video', { className: 'call-remote', autoplay: true, playsInline: true }),
    local: el('video', { className: 'call-local', autoplay: true, playsInline: true, muted: true, hidden: true }),
    photo: el('img', { className: 'call-photo', src: avatarURL(peer, 'large'), alt: '' }),
  };
  ui.stage = el('div', { className: 'call-stage' }, ui.photo, ui.remote, ui.local);
  ui.back = el('div', { className: 'call-back' },
    el('div', { className: 'call-window', role: 'dialog' },
      el('div', { className: 'call-title' },
        el('img', { src: '/static/icons/call-lock.svg', alt: '', width: 14, height: 14 }),
        el('span', { textContent: tr('Защищённый звонок') }),
        ui.bars),
      ui.stage,
      el('div', { className: 'call-name', textContent: fullName(peer) }),
      ui.status, ui.timer, ui.code, ui.buttons));
  document.body.append(ui.back);

  return ui;
}

function say(text) {
  if (call) call.ui.status.textContent = text;
}

// send hands signals on one after another: an address must not outrun the
// description it belongs to.
function send(c, signal) {
  c.line = c.line.then(async () => {
    const payload = JSON.stringify({ ...signal, id: c.id, tab, from: me.id });
    await api.callSignal(c.peer.nickname, await seal(me.id, c.peerKey, { t: payload, f: [] }));
  }).catch(() => {});

  return c.line;
}

function leavingGuard(event) {
  event.preventDefault();
}

function finish(text) {
  const c = call;
  if (!c) return;
  call = null;

  c.stopRing?.();
  clearTimeout(c.ringTimer);
  clearTimeout(c.goneTimer);
  clearInterval(c.clock);
  clearInterval(c.watch);
  c.pc?.close();
  for (const track of c.stream?.getTracks() ?? []) track.stop();
  c.camera?.stop();
  removeEventListener('beforeunload', leavingGuard);

  c.ui.buttons.replaceChildren();
  c.ui.status.textContent = text;
  setTimeout(() => c.ui.back.remove(), text ? SAY_FOR : 0);
}

function hangUp() {
  const c = call;
  if (!c) return;
  send(c, { k: c.started ? 'end' : 'cancel' });
  if (!c.started && c.role === 'caller') sendSealed(me.id, c.peer.nickname, tr('Пропущенный звонок')).catch(() => {});
  finish(tr('Звонок завершён'));
}

async function microphone() {
  return navigator.mediaDevices.getUserMedia({ audio: voice });
}

function refused(e) {
  if (e?.name === 'NotAllowedError') return tr('Разрешите браузеру доступ к микрофону.');
  if (e?.name === 'NotFoundError') return tr('Микрофон не найден.');

  return typeof e === 'string' ? e : tr('Не удалось начать звонок.');
}

function limit(sender, bitrate) {
  const parameters = sender.getParameters();
  if (!parameters.encodings?.length) return;
  parameters.encodings[0].maxBitrate = bitrate;
  sender.setParameters(parameters).catch(() => {});
}

// showCode puts the code of the call in the window: from the fingerprints
// of both sides, in the same order on both.
async function showCode(c) {
  const prints = [c.pc.localDescription?.sdp, c.pc.remoteDescription?.sdp]
    .map((sdp) => /a=fingerprint:\S+ (\S+)/.exec(sdp ?? '')?.[1] ?? '')
    .sort();
  const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(prints.join('|'))));
  const groups = [0, 2, 4].map((i) => String(((hash[i] << 8) | hash[i + 1]) % 10000).padStart(4, '0'));
  c.ui.code.textContent = tr('Код разговора: {0}. Если у собеседника такой же, между вами никого нет.', groups.join(' '));
  c.ui.code.hidden = false;
}

// watchLine shows how good the connection is by its round trip.
async function watchLine(c) {
  const stats = await c.pc.getStats();
  let trip = null;
  stats.forEach((s) => {
    if (s.type === 'candidate-pair' && s.nominated && s.currentRoundTripTime !== undefined) trip = s.currentRoundTripTime;
  });
  if (trip === null) return;

  const level = trip < 0.1 ? 4 : trip < 0.25 ? 3 : trip < 0.5 ? 2 : 1;
  [...c.ui.bars.children].forEach((bar, i) => bar.classList.toggle('on', i < level));
  c.ui.bars.hidden = false;
}

function connected(c) {
  clearTimeout(c.goneTimer);
  if (c.startedAt) {
    say(tr('Идёт разговор'));
    return;
  }

  c.startedAt = Date.now();
  say(tr('Идёт разговор'));
  c.ui.timer.hidden = false;
  c.ui.timer.textContent = timeOf(0);
  c.clock = setInterval(() => { c.ui.timer.textContent = timeOf(Math.floor((Date.now() - c.startedAt) / 1000)); }, 1000);
  c.watch = setInterval(() => watchLine(c).catch(() => {}), 2000);
  addEventListener('beforeunload', leavingGuard);
  showCode(c).catch(() => {});
  for (const t of c.pc.getTransceivers()) {
    if (t.sender.track?.kind === 'audio') limit(t.sender, AUDIO_BITRATE);
  }
}

async function makePeer(c) {
  const servers = await api.callServers();
  const iceServers = servers.urls.length ? [{ urls: servers.urls, username: servers.username, credential: servers.credential }] : [];
  const pc = new RTCPeerConnection({ iceServers, bundlePolicy: 'max-bundle' });
  const heard = new MediaStream();
  c.pc = pc;
  c.ui.remote.srcObject = heard;

  pc.ontrack = ({ track }) => heard.addTrack(track);
  pc.onicecandidate = ({ candidate }) => { if (candidate) send(c, { k: 'ice', candidate }); };
  pc.onnegotiationneeded = async () => {
    try {
      c.makingOffer = true;
      await pc.setLocalDescription();
      send(c, { k: 'sdp', description: pc.localDescription });
    } catch {
      // The next negotiation tries again.
    } finally {
      c.makingOffer = false;
    }
  };
  pc.onconnectionstatechange = () => {
    if (pc.connectionState === 'connected') connected(c);
    if (pc.connectionState !== 'disconnected' && pc.connectionState !== 'failed') return;

    say(tr('Связь прервалась, восстанавливаем…'));
    if (pc.connectionState === 'failed' && c.role === 'caller') pc.restartIce();
    clearTimeout(c.goneTimer);
    c.goneTimer = setTimeout(() => {
      if (call !== c) return;
      send(c, { k: 'end' });
      finish(tr('Связь потеряна'));
    }, GONE_FOR);
  };

  return pc;
}

// The camera goes on and off over a video line both sides have from the
// start: no new negotiation, so it switches at once.
async function toggleCamera(c, control) {
  const line = c.pc?.getTransceivers().find((t) => t.receiver.track.kind === 'video');
  if (!line) return;

  if (c.camera) {
    await line.sender.replaceTrack(null);
    c.camera.stop();
    c.camera = null;
    c.ui.local.hidden = true;
  } else {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: picture });
      [c.camera] = stream.getVideoTracks();
    } catch {
      say(tr('Разрешите браузеру доступ к камере.'));
      return;
    }
    await line.sender.replaceTrack(c.camera);
    limit(line.sender, VIDEO_BITRATE);
    c.ui.local.srcObject = new MediaStream([c.camera]);
    c.ui.local.hidden = false;
  }

  control.image.src = c.camera ? '/static/icons/call-camera.svg' : '/static/icons/call-camera-off.svg';
  send(c, { k: 'camera', on: Boolean(c.camera) });
}

function talkButtons(c) {
  const mic = iconButton('/static/icons/call-mic.svg', tr('Микрофон'), () => {
    const [track] = c.stream.getAudioTracks();
    track.enabled = !track.enabled;
    mic.image.src = track.enabled ? '/static/icons/call-mic.svg' : '/static/icons/call-mic-off.svg';
    mic.node.classList.toggle('off', !track.enabled);
  });
  const camera = iconButton('/static/icons/call-camera-off.svg', tr('Камера'), () => toggleCamera(c, camera));
  const end = iconButton('/static/icons/call-hangup.svg', tr('Завершить'), hangUp, 'call-end');
  c.ui.buttons.replaceChildren(mic.node, camera.node, end.node);
}

// startCall calls peer from this page.
export async function startCall(peer) {
  if (call) return;

  const ui = drawWindow(peer);
  const c = { id: crypto.randomUUID(), peer, role: 'caller', ui, line: Promise.resolve() };
  call = c;
  say(tr('Подключаемся…'));
  c.ui.buttons.replaceChildren(iconButton('/static/icons/call-hangup.svg', tr('Отмена'), hangUp, 'call-end').node);

  try {
    if (await ensureKey(me.id) !== KEY_READY) throw tr('На этом устройстве нет ключа шифрования сообщений: перенесите его с другого устройства в «Настройках».');
    const { publicKey } = await api.messageKey(peer.nickname);
    if (!publicKey) throw NO_PEER_KEY;
    c.peerKey = publicKey;
    c.stream = await microphone();
  } catch (e) {
    finish(refused(e));
    return;
  }
  if (call !== c) return;

  send(c, { k: 'ring' });
  say(tr('Вызываем…'));
  talkButtons(c);
  c.ringTimer = setTimeout(() => {
    if (call !== c || c.started) return;
    send(c, { k: 'cancel' });
    sendSealed(me.id, peer.nickname, tr('Пропущенный звонок')).catch(() => {});
    finish(tr('Нет ответа'));
  }, RING_FOR);
}

async function accept(c) {
  c.stopRing?.();
  clearTimeout(c.ringTimer);
  say(tr('Подключаемся…'));
  c.ui.buttons.replaceChildren();

  try {
    c.stream = await microphone();
    await makePeer(c);
  } catch (e) {
    send(c, { k: 'decline' });
    finish(refused(e));
    return;
  }

  c.started = true;
  talkButtons(c);
  send(c, { k: 'accept' });
}

function ring(peer, signal, peerKey) {
  const ui = drawWindow(peer);
  const c = { id: signal.id, peer, peerKey, peerTab: signal.tab, role: 'callee', ui, line: Promise.resolve() };
  call = c;
  ui.back.classList.add('call-ringing');
  say(tr('Вам звонят'));
  c.stopRing = ringer();
  c.ringTimer = setTimeout(() => { if (call === c) finish(tr('Пропущенный звонок')); }, RING_FOR);
  ui.buttons.replaceChildren(
    iconButton('/static/icons/call-phone.svg', tr('Принять'), () => accept(c), 'call-answer').node,
    iconButton('/static/icons/call-hangup.svg', tr('Отклонить'), () => {
      send(c, { k: 'decline' });
      finish(tr('Звонок отклонён'));
    }, 'call-end').node,
  );
}

// onDescription is perfect negotiation: the polite side gives way when
// both offer at once.
async function onDescription(c, description) {
  const { pc } = c;
  const collision = description.type === 'offer' && (c.makingOffer || pc.signalingState !== 'stable');
  if (collision && c.role === 'caller') return;

  await pc.setRemoteDescription(description);
  for (const candidate of c.earlyIce?.splice(0) ?? []) await pc.addIceCandidate(candidate).catch(() => {});
  if (description.type !== 'offer') return;

  if (!c.lines) {
    c.lines = true;
    const [line, video] = pc.getTransceivers();
    await line.sender.replaceTrack(c.stream.getAudioTracks()[0]);
    line.direction = 'sendrecv';
    if (video) video.direction = 'sendrecv';
  }
  await pc.setLocalDescription();
  send(c, { k: 'sdp', description: pc.localDescription });
}

async function onAccepted(c, signal) {
  clearTimeout(c.ringTimer);
  c.peerTab = signal.tab;
  c.started = true;
  say(tr('Подключаемся…'));
  const pc = await makePeer(c);
  pc.addTransceiver(c.stream.getAudioTracks()[0], { direction: 'sendrecv' });
  pc.addTransceiver('video', { direction: 'sendrecv' });
}

async function onPeerSignal(peer, signal, peerKey) {
  if (signal.k === 'ring') {
    if (call) {
      send({ id: signal.id, peer, peerKey, line: Promise.resolve() }, { k: 'busy' });
      return;
    }
    ring(peer, signal, peerKey);
    return;
  }

  const c = call;
  if (!c || c.id !== signal.id) return;
  // Once answered, only the page that answered speaks.
  if (c.peerTab && c.peerTab !== signal.tab) return;

  switch (signal.k) {
    case 'accept':
      if (c.role === 'caller' && !c.started) await onAccepted(c, signal);
      break;
    case 'decline':
      finish(tr('Звонок отклонён'));
      break;
    case 'busy':
      finish(tr('Абонент занят'));
      break;
    case 'cancel':
      finish(tr('Пропущенный звонок'));
      break;
    case 'end':
      finish(tr('Звонок завершён'));
      break;
    case 'sdp':
      if (c.pc) await onDescription(c, signal.description);
      break;
    case 'ice':
      // An address can overtake the description it belongs to.
      if (c.pc?.remoteDescription) await c.pc.addIceCandidate(signal.candidate).catch(() => {});
      else (c.earlyIce ??= []).push(signal.candidate);
      break;
    case 'camera':
      c.ui.stage.classList.toggle('call-seen', Boolean(signal.on));
      break;
    default:
  }
}

// A signal of a person's other page: the call was answered or turned down
// there, so this page stops ringing.
function onOwnSignal(signal) {
  if (call?.id !== signal.id || call.role !== 'callee' || call.started) return;
  if (signal.k === 'accept' || signal.k === 'decline') finish(tr('Ответили на другой странице'));
}

async function onSignal(event) {
  if (!me) return;

  const opened = await unseal(me.id, event.sealed);
  if (!opened) return;

  const signal = JSON.parse(opened.t);
  if (signal.tab === tab) return;
  if (signal.from === me.id) {
    onOwnSignal(signal);
    return;
  }
  if (signal.from !== event.peer.id) return;

  const peerKey = call?.peer.id === event.peer.id ? call.peerKey : (await api.messageKey(event.peer.nickname)).publicKey;
  await onPeerSignal(event.peer, signal, peerKey);
}

// A queue of its own: signals open one after another, in the order they
// came, however long each takes.
let signals = Promise.resolve();

// listenCalls makes this page answer calls of me.
export function listenCalls(user) {
  if (me) return;
  me = user;
  onEvent('call_signal', (event) => {
    signals = signals.then(() => onSignal(event)).catch(() => {});
  });
}
