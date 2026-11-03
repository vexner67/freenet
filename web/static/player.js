// The player of songs, as openvk's mini player (#ajax_audio_player): a
// small dark box over the corner of the page — ▶, "Исполнитель — Название"
// with the time, the line of the song under them, the line of loudness,
// back, repeat, shuffle and forward, and × in its corner; the list it
// plays opens over it under the pointer. On a phone it is a strip over the
// bar of sections.
//
// An uploaded MP3 plays in an <audio> of its own. A track of SoundCloud or
// YouTube plays in the player of its site, kept out of sight and told what
// to do by postMessage, as their widget APIs have it — no scripts of
// theirs run here: the time, the line and the loudness are ours. A song
// that does not play is grey, "Трек недоступен", and the next one plays.
//
// What plays, where in it and how loud is kept for the tab
// (sessionStorage): a page opened anew (a reload) opens the player where
// the last one left it. Pages opened from it do not stop the song at all:
// they open in the frame of shell.js.

import { api, signedIn } from './api.js';
import { audioArtist, audioSite, audioTime, audioTitle } from './labels.js';
import { el } from './layout.js';
import { tr } from './i18n.js';

const SOUNDCLOUD = 'https://w.soundcloud.com';
const YOUTUBE = 'https://www.youtube-nocookie.com';
// How long "Трек недоступен" shows before the next song plays.
const SKIP_AFTER = 1500;
const STORE = 'freenet.player';
// How many songs of the list the tab keeps for the next page.
const KEEP = 300;

// youTubeID is the video of a link the server made canonical
// ("https://www.youtube.com/watch?v=<id>").
function youTubeID(link) {
  try {
    return new URL(link).searchParams.get('v') ?? '';
  } catch {
    return '';
  }
}

function embedURL(audio, { autoplay, from }) {
  if (audio.kind === 'soundcloud') {
    const query = new URLSearchParams({
      url: audio.link, auto_play: String(autoplay), hide_related: 'true', show_comments: 'false',
      show_reposts: 'false', show_teaser: 'false', visual: 'false',
    });
    return `${SOUNDCLOUD}/player/?${query}`;
  }

  const query = new URLSearchParams({ autoplay: autoplay ? '1' : '0', enablejsapi: '1', playsinline: '1', rel: '0', origin: location.origin });
  if (from > 0) query.set('start', String(Math.floor(from)));
  return `${YOUTUBE}/embed/${encodeURIComponent(youTubeID(audio.link))}?${query}`;
}

// saved is what the last page of the tab played, null for nothing.
export function saved() {
  try {
    const state = JSON.parse(sessionStorage.getItem(STORE) ?? 'null');
    return Array.isArray(state?.queue) && state.queue[state.index] ? state : null;
  } catch {
    return null;
  }
}

function forget() {
  try {
    sessionStorage.removeItem(STORE);
  } catch {
    // Nothing kept, nothing to forget.
  }
}

function iconButton(className, label, onclick) {
  return el('button', { type: 'button', className: 'player-btn ' + className, title: label, ariaLabel: label, onclick });
}

// createPlayer puts the player into root. onState hears what plays (the
// id, whether it plays, the songs that would not); onStop hears the ×.
export function createPlayer(root, { onState = () => {}, onStop = () => {} } = {}) {
  const media = new Audio();
  media.preload = 'auto';

  let queue = [];
  let index = -1;
  let on = false;
  let frame = null;
  let told = '';
  let position = 0;
  let length = 0;
  let left = false;
  let shuffle = false;
  let repeat = false;
  let loudness = 80;
  // resumeAt is where a song restored from the last page goes on from.
  let resumeAt = 0;
  const broken = new Set();

  const artist = el('b');
  const title = el('span');
  const gone = el('span', { className: 'player-broken', hidden: true, textContent: tr('Трек недоступен') });
  const name = el('a', { className: 'player-name' }, artist, ' — ', title, gone);
  const time = el('span', {
    className: 'player-time', title: tr('Сколько осталось'), textContent: '0:00',
    onclick: () => {
      left = !left;
      showTime();
    },
  });
  const seek = el('input', { type: 'range', className: 'player-seek', min: 0, max: 1000, value: 0, ariaLabel: tr('Перемотка') });
  const volume = el('input', { type: 'range', className: 'player-volume', min: 0, max: 100, value: 80, ariaLabel: tr('Громкость') });
  const toggle = iconButton('player-toggle', tr('Слушать'), () => togglePlay());
  const prev = iconButton('player-prev', tr('Предыдущая'), () => step(-1));
  const next = iconButton('player-next', tr('Следующая'), () => step(1));
  const repeatButton = iconButton('player-repeat', tr('Повторять песню'), () => setRepeat(!repeat));
  const shuffleButton = iconButton('player-shuffle', tr('Перемешать'), () => setShuffle(!shuffle));
  const list = el('div', { className: 'player-queue' });
  const listButton = iconButton('player-list', tr('Список воспроизведения'), () => box.classList.toggle('queue-open'));
  const close = el('button', { type: 'button', className: 'player-close', title: tr('Закрыть'), ariaLabel: tr('Закрыть'), onclick: stop });
  // The player of SoundCloud or YouTube: it plays, but is not seen.
  const embed = el('div', { className: 'player-embed', ariaHidden: 'true' });

  const box = el('div', { className: 'player', hidden: true },
    list,
    el('div', { className: 'player-main' },
      toggle,
      el('div', { className: 'player-track' },
        el('div', { className: 'player-line' }, name, time),
        seek),
      volume,
      el('div', { className: 'player-buttons' }, prev, repeatButton, shuffleButton, next, listButton)),
    close,
    embed);
  root.append(box);

  volume.addEventListener('input', () => setLoudness(Number(volume.value)));
  seek.addEventListener('input', () => seekTo((Number(seek.value) / 1000) * length));
  media.addEventListener('timeupdate', () => {
    position = media.currentTime;
    showTime();
  });
  media.addEventListener('loadedmetadata', () => {
    if (Number.isFinite(media.duration)) length = media.duration;
    if (resumeAt > 0 && resumeAt < length) media.currentTime = resumeAt;
    resumeAt = 0;
    showTime();
  });
  media.addEventListener('playing', () => setOn(true));
  media.addEventListener('pause', () => setOn(false));
  media.addEventListener('ended', () => ended());
  media.addEventListener('error', () => {
    if (current()?.kind === 'file' && media.getAttribute('src')) fail();
  });

  window.addEventListener('message', onFrameMessage);
  window.addEventListener('pagehide', keep);
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') box.classList.remove('queue-open');
  });
  document.addEventListener('click', (event) => {
    if (!box.contains(event.target)) box.classList.remove('queue-open');
  });

  const current = () => queue[index] ?? null;

  function showTime() {
    time.textContent = left && length ? '-' + audioTime(Math.max(0, length - position)) : audioTime(position);
    seek.value = length ? String(Math.round((position / length) * 1000)) : '0';
  }

  function setOn(value) {
    on = value;
    toggle.classList.toggle('on', on);
    toggle.title = on ? tr('Пауза') : tr('Слушать');
    toggle.ariaLabel = toggle.title;
    if (on) tell();
    say();
  }

  function setShuffle(value) {
    shuffle = value;
    shuffleButton.classList.toggle('active', shuffle);
    shuffleButton.ariaPressed = String(shuffle);
  }

  function setRepeat(value) {
    repeat = value;
    repeatButton.classList.toggle('active', repeat);
    repeatButton.ariaPressed = String(repeat);
  }

  function setLoudness(value) {
    loudness = value;
    volume.value = String(value);
    media.volume = value / 100;
    const audio = current();
    if (audio?.kind === 'soundcloud') post({ method: 'setVolume', value });
    if (audio?.kind === 'youtube') post({ event: 'command', func: 'setVolume', args: [value] });
  }

  function seekTo(at) {
    const audio = current();
    if (!audio || !length) return;

    position = at;
    showTime();
    if (audio.kind === 'file') media.currentTime = at;
    else if (audio.kind === 'soundcloud') post({ method: 'seekTo', value: Math.round(at * 1000) });
    else post({ event: 'command', func: 'seekTo', args: [at, true] });
  }

  // tell is "слушает: …" under the name of the one who listens, once a
  // song; a guest has no name to show it under.
  function tell() {
    const audio = current();
    if (!audio || told === audio.id || !signedIn()) return;

    told = audio.id;
    api.listen(audio.id).catch(() => {});
  }

  // say tells the pages what plays.
  function say() {
    onState({ id: current()?.id ?? '', on, broken: [...broken] });
  }

  // keep leaves what plays to the next page of the tab.
  function keep() {
    if (!current()) return;

    const from = Math.max(0, index - 50);
    try {
      sessionStorage.setItem(STORE, JSON.stringify({
        queue: queue.slice(from, from + KEEP), index: index - from, position, on, told,
        shuffle, repeat, loudness, broken: [...broken],
      }));
    } catch {
      // A tab that keeps nothing starts with no song.
    }
  }

  function show(audio) {
    artist.textContent = audioArtist(audio);
    title.textContent = audioTitle(audio);
    gone.hidden = !broken.has(audio.id);
    // A track names its site: the name opens it there.
    if (audio.link) {
      name.href = audio.link;
      name.target = '_blank';
      name.rel = 'noopener noreferrer';
      name.title = tr('Слушать на {0}', audioSite(audio));
    } else {
      name.removeAttribute('href');
      name.removeAttribute('title');
    }
    position = 0;
    length = audio.duration || 0;
    showTime();

    if ('mediaSession' in navigator && typeof MediaMetadata === 'function') {
      navigator.mediaSession.metadata = new MediaMetadata({
        artist: audioArtist(audio), title: audioTitle(audio), artwork: audio.cover ? [{ src: audio.cover }] : [],
      });
    }

    for (const row of list.children) {
      row.classList.toggle('playing', row.dataset.audio === audio.id);
      row.classList.toggle('broken', broken.has(row.dataset.audio));
    }
  }

  // start plays the song i of the list; autoplay false only shows it,
  // from is where it goes on (a song of the last page).
  function start(i, { autoplay = true, from = 0 } = {}) {
    index = i;
    const audio = current();
    if (!audio) return;

    box.hidden = false;
    document.body.classList.add('with-player');
    show(audio);
    media.pause();
    frame?.remove();
    frame = null;
    resumeAt = from;

    if (audio.kind === 'file') {
      media.src = audio.url;
      // A page that opened without a click may not play by itself: then ▶
      // waits for one.
      if (autoplay) media.play().catch(() => setOn(false));
      else setOn(false);
    } else {
      media.removeAttribute('src');
      media.load();
      frame = el('iframe', {
        src: embedURL(audio, { autoplay, from }),
        allow: 'autoplay; encrypted-media',
        referrerPolicy: 'strict-origin-when-cross-origin',
        title: `${audioArtist(audio)} — ${audioTitle(audio)}`,
        tabIndex: -1,
      });
      frame.addEventListener('load', () => listenTo(audio));
      embed.append(frame);
      setOn(false);
    }
    say();
  }

  // listenTo asks the player of YouTube to tell what it does (SoundCloud's
  // says it is ready by itself).
  function listenTo(audio) {
    if (audio.kind === 'youtube') {
      post({ event: 'listening', id: 'freenet', channel: 'widget' });
      post({ event: 'command', func: 'addEventListener', args: ['onStateChange'] });
      post({ event: 'command', func: 'addEventListener', args: ['onError'] });
      post({ event: 'command', func: 'setVolume', args: [loudness] });
    }
  }

  function post(message) {
    const audio = current();
    if (!frame || !audio) return;

    frame.contentWindow?.postMessage(JSON.stringify(message), audio.kind === 'youtube' ? YOUTUBE : SOUNDCLOUD);
  }

  function onFrameMessage(event) {
    if (!frame || event.source !== frame.contentWindow) return;
    if (event.origin !== SOUNDCLOUD && event.origin !== YOUTUBE) return;

    let data;
    try {
      data = typeof event.data === 'string' ? JSON.parse(event.data) : event.data;
    } catch {
      return;
    }
    if (!data || typeof data !== 'object') return;

    if (event.origin === SOUNDCLOUD) onSoundCloud(data);
    else onYouTube(data);
  }

  function onSoundCloud({ method, value }) {
    if (method === 'ready') {
      for (const event of ['play', 'pause', 'finish', 'error', 'playProgress']) post({ method: 'addEventListener', value: event });
      post({ method: 'getDuration' });
      post({ method: 'setVolume', value: loudness });
      if (resumeAt > 0) post({ method: 'seekTo', value: resumeAt * 1000 });
      resumeAt = 0;
    }
    if (method === 'getDuration' && value > 0) {
      length = value / 1000;
      showTime();
    }
    if (method === 'playProgress' && typeof value?.currentPosition === 'number') {
      position = value.currentPosition / 1000;
      if (!length && value.relativePosition > 0) length = position / value.relativePosition;
      showTime();
    }
    if (method === 'play') setOn(true);
    if (method === 'pause') setOn(false);
    if (method === 'finish') ended();
    if (method === 'error') fail();
  }

  function onYouTube({ event, info }) {
    const state = event === 'onStateChange' ? info : event === 'infoDelivery' ? info?.playerState : undefined;
    if (event === 'infoDelivery') {
      if (info?.duration > 0) length = info.duration;
      if (typeof info?.currentTime === 'number') position = info.currentTime;
      showTime();
    }
    if (state === 1) setOn(true);
    if (state === 2) setOn(false);
    if (state === 0) ended();
    if (event === 'onError') fail();
  }

  // fail greys a song that does not play and goes on to the next.
  function fail() {
    const audio = current();
    if (!audio || broken.has(audio.id)) return;

    broken.add(audio.id);
    setOn(false);
    show(audio);
    const was = index;
    setTimeout(() => {
      if (index === was) step(1, true);
    }, SKIP_AFTER);
  }

  // ended is a song over: the same once more on repeat, the next one
  // otherwise.
  function ended() {
    if (repeat && !broken.has(current()?.id)) {
      start(index);
      return;
    }
    step(1, true);
  }

  // step plays the song by, skipping those that do not play — on shuffle
  // any other one; at the end of the list (ended: the last one is over)
  // it stops.
  function step(by, done = false) {
    if (shuffle) {
      const rest = queue.map((_, i) => i).filter((i) => i !== index && !broken.has(queue[i].id));
      if (rest.length) {
        start(rest[Math.floor(Math.random() * rest.length)]);
        return;
      }
    } else {
      for (let i = index + by; i >= 0 && i < queue.length; i += by) {
        if (!broken.has(queue[i].id)) {
          start(i);
          return;
        }
      }
    }
    if (done) {
      setOn(false);
      if (signedIn()) api.listen('').catch(() => {});
      told = '';
    }
  }

  function togglePlay() {
    const audio = current();
    if (!audio) return;

    if (audio.kind === 'file') {
      if (media.paused) media.play().catch(() => setOn(false));
      else media.pause();
    } else if (audio.kind === 'soundcloud') {
      post({ method: on ? 'pause' : 'play' });
    } else {
      post({ event: 'command', func: on ? 'pauseVideo' : 'playVideo', args: [] });
    }
  }

  function stop() {
    media.pause();
    media.removeAttribute('src');
    frame?.remove();
    frame = null;
    index = -1;
    queue = [];
    box.hidden = true;
    box.classList.remove('queue-open');
    document.body.classList.remove('with-player');
    setOn(false);
    forget();
    if (signedIn()) api.listen('').catch(() => {});
    told = '';
    onStop();
  }

  function renderQueue() {
    list.replaceChildren(...queue.map((a, i) => {
      const row = el('a', { className: 'player-row', onclick: () => start(i) },
        el('b', { textContent: audioArtist(a) }), ' — ', el('span', { textContent: audioTitle(a) }));
      row.dataset.audio = a.id;
      return row;
    }));
  }

  function play(audio, songs = [audio]) {
    queue = songs.some((a) => a.id === audio.id) ? songs.slice(0, 1000) : [audio];
    renderQueue();
    start(queue.findIndex((a) => a.id === audio.id));
  }

  // restore opens the player where the last page of the tab left it.
  function restore(state) {
    queue = state.queue;
    for (const id of state.broken ?? []) broken.add(id);
    told = state.told ?? '';
    setShuffle(Boolean(state.shuffle));
    setRepeat(Boolean(state.repeat));
    setLoudness(Number.isFinite(state.loudness) ? state.loudness : 80);
    renderQueue();
    start(state.index, { autoplay: Boolean(state.on), from: Number(state.position) || 0 });
  }

  if ('mediaSession' in navigator) {
    navigator.mediaSession.setActionHandler('previoustrack', () => step(-1));
    navigator.mediaSession.setActionHandler('nexttrack', () => step(1));
  }

  return { play, toggle: togglePlay, restore, active: () => Boolean(current()) };
}
