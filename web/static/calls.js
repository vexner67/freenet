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
import { ensureKey, KEY_READY, NO_PEER_KEY, open as unseal, seal } from './e2ee.js';
import { onEvent } from './events.js';
import { helpButton } from './game.js';
import { tr } from './i18n.js';
import { avatarURL, el, fullName } from './layout.js';
import { callClick, callDown, callUp } from './sounds.js';

const RING_FOR = 45_000;
const CONNECT_FOR = 30_000;
const GONE_FOR = 20_000;
const SAY_FOR = 2_000;
const AUDIO_BITRATE = 64_000;
const VIDEO_BITRATE = 1_500_000;

const voice = { echoCancellation: true, noiseSuppression: true, autoGainControl: true };
const picture = { width: { ideal: 1280 }, height: { ideal: 720 }, frameRate: { ideal: 30 } };
const tab = crypto.randomUUID();
const still = matchMedia('(prefers-reduced-motion: reduce)').matches;

let me = null;
let call = null;

function timeOf(seconds) {
  const part = (n) => String(n).padStart(2, '0');
  const hours = Math.floor(seconds / 3600);

  return `${hours ? `${hours}:` : ''}${part(Math.floor(seconds / 60) % 60)}:${part(seconds % 60)}`;
}

// beeps plays notes — [start, pitch, length] in seconds — again every
// period, until the returned function stops them. The browser draws them:
// no sound file to fetch. A page the person has not touched may not play
// them; the window and the title still show the call.
function beeps(notes, period, vibrate = null) {
  const AudioContext = window.AudioContext ?? window.webkitAudioContext;
  if (!AudioContext) return () => {};

  const audio = new AudioContext();
  const play = () => {
    for (const [at, pitch, length] of notes) {
      const tone = audio.createOscillator();
      const gain = audio.createGain();
      tone.frequency.value = pitch;
      gain.gain.setValueAtTime(0.0001, audio.currentTime + at);
      gain.gain.exponentialRampToValueAtTime(0.12, audio.currentTime + at + 0.03);
      gain.gain.setValueAtTime(0.12, audio.currentTime + at + length - 0.05);
      gain.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + at + length);
      tone.connect(gain).connect(audio.destination);
      tone.start(audio.currentTime + at);
      tone.stop(audio.currentTime + at + length + 0.02);
    }
    if (vibrate) navigator.vibrate?.(vibrate);
  };
  play();
  const timer = setInterval(play, period);

  return () => {
    clearInterval(timer);
    audio.close().catch(() => {});
  };
}

// ringer is the two-tone ring of a call coming in.
const ringer = () => beeps([[0, 620, 0.4], [0.45, 520, 0.4], [0.9, 620, 0.4], [1.35, 520, 0.4]], 3500, [400, 200, 400]);

// ringback is what the caller hears while the other side rings: the long
// tone of a telephone line, a second of 425 Hz every four.
const ringback = () => beeps([[0, 425, 1]], 4000);

// iconButton is a round glossy button with a caption under it.
function iconButton(icon, label, onclick, className = '') {
  const image = el('img', { src: icon, alt: '', width: 30, height: 30 });
  const disc = el('span', { className: 'call-disc' }, image);
  const caption = el('span', { className: 'call-caption', textContent: label });
  const node = el('button', { type: 'button', className: `call-button ${className}`, onclick }, disc, caption);

  return { node, disc, image, caption };
}

// The window: a dark title with a lock and the bars of the line, the person
// with waves around them while it rings and a glow that beats with their
// voice while they talk, a plaque of how it goes next to the clock, the
// code of the call and a row of big round buttons. Everything has its
// place from the start, so nothing jumps while the call goes on.
function drawWindow(peer) {
  const ui = {
    status: el('span', { className: 'call-status-text' }),
    clock: el('b', { className: 'call-clock-digits', textContent: timeOf(0) }),
    bars: el('span', { className: 'call-bars' }, ...[1, 2, 3, 4].map(() => el('i'))),
    digits: el('span', { className: 'call-digits' }),
    buttons: el('div', { className: 'call-buttons' }),
    glow: el('span', { className: 'call-glow' }),
    sound: el('audio', { autoplay: true }),
    remote: el('video', { className: 'call-remote', autoplay: true, playsInline: true, muted: true }),
    local: el('video', { className: 'call-local', autoplay: true, playsInline: true, muted: true, hidden: true }),
    photo: el('img', { className: 'call-photo', src: avatarURL(peer, 'large'), alt: '' }),
  };
  ui.code = el('div', { className: 'call-code' },
    el('span', { className: 'call-code-name', textContent: tr('Код') }), ui.digits,
    helpButton(tr('Если у собеседника такой же код, между вами никого нет: голос и видео шифруются от браузера до браузера.')));
  ui.stage = el('div', { className: 'call-stage' },
    el('span', { className: 'call-waves' }, el('i'), el('i'), el('i')),
    ui.glow,
    el('span', { className: 'call-frame' }, ui.photo),
    ui.remote, ui.local, ui.sound);
  ui.window = el('div', { className: 'call-window', role: 'dialog', ariaLabel: tr('Защищённый звонок') },
    el('div', { className: 'call-title' },
      el('img', { src: '/static/icons/call-lock.svg', alt: '', width: 16, height: 16 }),
      el('span', { textContent: tr('Защищённый звонок') }),
      ui.bars),
    ui.stage,
    el('div', { className: 'call-name', textContent: fullName(peer) }),
    el('div', { className: 'call-plaques' },
      el('span', { className: 'call-status' }, el('i', { className: 'call-lamp' }), ui.status),
      el('span', { className: 'call-clock' }, ui.clock)),
    ui.code, ui.buttons);
  ui.back = el('div', { className: 'call-back' }, ui.window);
  document.body.append(ui.back);

  return ui;
}

// mood is what the window shows around the person: "calling" and "ringing"
// send waves, "live" lights the glow and the lamp, "trouble" makes the lamp
// blink.
function mood(c, name) {
  c.ui.back.classList.remove('call-calling', 'call-ringing', 'call-live', 'call-trouble');
  if (name) c.ui.back.classList.add(`call-${name}`);
}

function say(text) {
  if (call) call.ui.status.textContent = text;
}

// send hands signals on one after another: an address must not outrun the
// description it belongs to. It resolves to whether the signal went.
function send(c, signal) {
  const sent = c.line.then(async () => {
    const payload = JSON.stringify({ ...signal, id: c.id, tab, from: me.id });
    await api.callSignal(c.peer.nickname, await seal(me.id, c.peerKey, { t: payload, f: [] }));
  });
  c.line = sent.catch(() => {});

  return sent.then(() => true, () => false);
}

function leavingGuard(event) {
  event.preventDefault();
}

// report tells the server how the call of the caller ended: the seconds
// they talked, 0 when the peer did not answer. Both see it in
// "Уведомления"; the dialog keeps only the button of a call.
function report(c, seconds) {
  if (c.role === 'caller') api.reportCall(c.peer.nickname, seconds).catch(() => {});
}

function finish(text) {
  const c = call;
  if (!c) return;
  call = null;
  if (c.startedAt) report(c, Math.max(1, Math.round((Date.now() - c.startedAt) / 1000)));

  c.stopRing?.();
  clearTimeout(c.ringTimer);
  clearTimeout(c.goneTimer);
  clearTimeout(c.connectTimer);
  clearInterval(c.clock);
  clearInterval(c.watch);
  cancelAnimationFrame(c.meter);
  c.audio?.close().catch(() => {});
  c.pc?.close();
  for (const track of c.stream?.getTracks() ?? []) track.stop();
  c.camera?.stop();
  removeEventListener('beforeunload', leavingGuard);
  if (c.startedAt) callDown();

  mood(c, '');
  c.ui.back.classList.add('call-over');
  for (const button of c.ui.buttons.querySelectorAll('button')) button.disabled = true;
  c.ui.status.textContent = text;
  setTimeout(() => c.ui.back.remove(), text ? SAY_FOR : 0);
}

function hangUp() {
  const c = call;
  if (!c) return;
  send(c, { k: c.started ? 'end' : 'cancel' });
  if (!c.started) report(c, 0);
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
  c.ui.digits.replaceChildren(...groups.map((group) => el('b', { textContent: group })));
  c.ui.code.classList.add('shown');
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
}

// loudness is how loud a voice is now, from 0 to 1.
function loudness(analyser, samples) {
  analyser.getByteTimeDomainData(samples);
  let sum = 0;
  for (const sample of samples) sum += ((sample - 128) / 128) ** 2;

  return Math.min(1, Math.sqrt(sum / samples.length) * 4);
}

// meter makes the glow around the person beat with their voice and the bar
// on the microphone button rise with one's own: a call that is alive looks
// alive. Only transform and opacity change.
function meter(c) {
  if (!c.audio) return;
  const analyser = (stream) => {
    const node = c.audio.createAnalyser();
    node.fftSize = 512;
    c.audio.createMediaStreamSource(stream).connect(node);
    return node;
  };
  const mine = c.stream ? analyser(c.stream) : null;
  let theirs = null;
  const samples = new Uint8Array(512);
  let heard = 0;
  let said = 0;

  const step = () => {
    if (call !== c) return;
    if (!theirs && c.heard?.getAudioTracks().length) theirs = analyser(new MediaStream(c.heard.getAudioTracks()));
    heard = Math.max(theirs ? loudness(theirs, samples) : 0, heard * 0.85);
    said = Math.max(mine ? loudness(mine, samples) : 0, said * 0.8);
    c.ui.glow.style.transform = `scale(${1 + heard * 0.22})`;
    c.ui.glow.style.opacity = String(0.35 + heard * 0.65);
    if (c.level) c.level.style.transform = `scaleY(${c.muted ? 0 : said})`;
    c.meter = requestAnimationFrame(step);
  };
  c.audio.resume().catch(() => {});
  step();
}

// tick runs every second of a call: the clock.
function tick(c) {
  c.ui.clock.textContent = timeOf(Math.floor((Date.now() - c.startedAt) / 1000));
}

function connected(c) {
  clearTimeout(c.goneTimer);
  clearTimeout(c.connectTimer);
  mood(c, 'live');
  say(tr('Идёт разговор'));
  if (c.startedAt) return;

  c.startedAt = Date.now();
  callUp();
  c.ui.clock.classList.add('on');
  c.clock = setInterval(() => tick(c), 1000);
  c.watch = setInterval(() => watchLine(c).catch(() => {}), 2000);
  addEventListener('beforeunload', leavingGuard);
  showCode(c).catch(() => {});
  meter(c);
  for (const t of c.pc.getTransceivers()) {
    if (t.sender.track?.kind === 'audio') limit(t.sender, AUDIO_BITRATE);
  }
}

// waitConnection ends a call whose browsers never reach each other, with a
// word of why, instead of leaving it "connecting" for ever.
function waitConnection(c) {
  clearTimeout(c.connectTimer);
  c.connectTimer = setTimeout(() => {
    if (call !== c || c.startedAt) return;
    send(c, { k: 'end' });
    finish(tr('Не удалось соединиться. Проверьте интернет и попробуйте ещё раз.'));
  }, CONNECT_FOR);
}

// onTrack plays what comes from the other side: the voice through an audio
// element of its own, so it sounds while no picture is shown, and the
// picture in the stage.
function onTrack(c, track) {
  c.heard.addTrack(track);
  const element = track.kind === 'audio' ? c.ui.sound : c.ui.remote;
  element.srcObject = new MediaStream(c.heard.getTracks().filter((t) => t.kind === track.kind));
  element.play().catch(() => {});
}

function lineChanged(c) {
  const { pc } = c;
  if (pc.connectionState === 'connected') {
    connected(c);
    return;
  }
  if (pc.connectionState !== 'disconnected' && pc.connectionState !== 'failed') return;

  mood(c, 'trouble');
  say(tr('Связь прервалась, восстанавливаем…'));
  // Either side may ask for new addresses: perfect negotiation sorts out
  // the two asking at once.
  if (pc.connectionState === 'failed') pc.restartIce();
  clearTimeout(c.goneTimer);
  c.goneTimer = setTimeout(() => {
    if (call !== c) return;
    send(c, { k: 'end' });
    finish(tr('Связь потеряна'));
  }, GONE_FOR);
}

async function makePeer(c) {
  const servers = await api.callServers();
  const iceServers = servers.urls.length ? [{ urls: servers.urls, username: servers.username, credential: servers.credential }] : [];
  const pc = new RTCPeerConnection({ iceServers, bundlePolicy: 'max-bundle', iceCandidatePoolSize: 2 });
  c.pc = pc;
  c.heard = new MediaStream();

  pc.ontrack = ({ track }) => onTrack(c, track);
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
  pc.onconnectionstatechange = () => lineChanged(c);
  waitConnection(c);

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

  callClick(Boolean(c.camera));
  control.image.src = c.camera ? '/static/icons/call-camera.svg' : '/static/icons/call-camera-off.svg';
  control.node.classList.toggle('on', Boolean(c.camera));
  send(c, { k: 'camera', on: Boolean(c.camera) });
}

function talkButtons(c) {
  const mic = iconButton('/static/icons/call-mic.svg', tr('Микрофон'), () => {
    const [track] = c.stream.getAudioTracks();
    track.enabled = !track.enabled;
    c.muted = !track.enabled;
    callClick(track.enabled);
    mic.image.src = track.enabled ? '/static/icons/call-mic.svg' : '/static/icons/call-mic-off.svg';
    mic.node.classList.toggle('off', !track.enabled);
  });
  c.level = el('i', { className: 'call-level' });
  mic.disc.append(c.level);
  const camera = iconButton('/static/icons/call-camera-off.svg', tr('Камера'), () => toggleCamera(c, camera));
  const end = iconButton('/static/icons/call-hangup.svg', tr('Завершить'), hangUp, 'call-end');
  c.ui.buttons.replaceChildren(mic.node, end.node, camera.node);
}

// voiceMeter is the audio context that measures the voices of a call. It
// is made on a press, as browsers want; with reduced motion nothing beats,
// so there is none.
function voiceMeter() {
  const AudioContext = window.AudioContext ?? window.webkitAudioContext;

  return AudioContext && !still ? new AudioContext() : null;
}

// startCall calls peer from this page.
export async function startCall(peer) {
  if (call) return;

  const ui = drawWindow(peer);
  const c = { id: crypto.randomUUID(), peer, role: 'caller', ui, line: Promise.resolve(), audio: voiceMeter() };
  call = c;
  mood(c, 'calling');
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

  talkButtons(c);
  if (!await send(c, { k: 'ring' })) {
    finish(tr('Не удалось дозвониться. Попробуйте ещё раз.'));
    return;
  }
  if (call !== c) return;
  say(tr('Вызываем…'));
  c.stopRing = ringback();
  c.ringTimer = setTimeout(() => {
    if (call !== c || c.started) return;
    send(c, { k: 'cancel' });
    report(c, 0);
    finish(tr('Нет ответа'));
  }, RING_FOR);
}

async function accept(c) {
  c.stopRing?.();
  clearTimeout(c.ringTimer);
  mood(c, 'calling');
  say(tr('Соединяемся…'));
  c.audio = voiceMeter();
  for (const button of c.ui.buttons.querySelectorAll('button')) button.disabled = true;

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
  if (!await send(c, { k: 'accept' })) finish(tr('Не удалось соединиться. Проверьте интернет и попробуйте ещё раз.'));
}

function ring(peer, signal, peerKey) {
  const ui = drawWindow(peer);
  const c = { id: signal.id, peer, peerKey, peerTab: signal.tab, role: 'callee', ui, line: Promise.resolve() };
  call = c;
  mood(c, 'ringing');
  say(tr('Вам звонят'));
  c.stopRing = ringer();
  c.ringTimer = setTimeout(() => { if (call === c) finish(tr('Пропущенный звонок')); }, RING_FOR);
  ui.buttons.replaceChildren(
    iconButton('/static/icons/call-hangup.svg', tr('Отклонить'), () => {
      send(c, { k: 'decline' });
      finish(tr('Звонок отклонён'));
    }, 'call-end').node,
    iconButton('/static/icons/call-phone.svg', tr('Принять'), () => accept(c), 'call-answer').node,
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
  c.stopRing?.();
  c.peerTab = signal.tab;
  c.started = true;
  say(tr('Соединяемся…'));
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
