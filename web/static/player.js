// The player of songs: a bar over the bottom of the page, in the tab the
// song was started in, with the buttons of a player of the 2000s — back,
// a big ▶, forward, the song with its cover, the time line, shuffle,
// repeat, loudness, the list it plays and ×. An uploaded MP3 plays in an
// <audio> of its own; a track of SoundCloud or YouTube plays in the
// player of its site, shown over the bar and told what to do by
// postMessage, as their widget APIs have it — no scripts of theirs run
// here. A song that does not play is grey, "Трек недоступен", and the
// next one plays.
//
// What plays, where in it and how loud is kept for the tab
// (sessionStorage): the next page of the site opens the bar where the
// last one left it and plays on, if the browser lets it play by itself.

import { api, signedIn } from './api.js';
import { audioArtist, audioSite, audioTime, audioTitle } from './labels.js';
import { el, externalLink } from './layout.js';
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
      show_reposts: 'false', show_teaser: 'false', visual: 'false', color: '#595959',
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

// iconButton is a round glossy button of the bar.
function iconButton(className, label, onclick) {
  return el('button', { type: 'button', className: 'player-btn ' + className, title: label, ariaLabel: label, onclick });
}

// createPlayer puts the bar into root; onState hears what plays (the id,
// whether it plays, the songs that would not).
export function createPlayer(root, { onState = () => {} } = {}) {
  const media = new Audio();
  media.preload = 'auto';

  let queue = [];
  let index = -1;
  let on = false;
  let frame = null;
  let told = '';
  let position = 0;
  let shuffle = false;
  let repeat = false;
  let muted = false;
  let loudness = 80;
  // resumeAt is where a song restored from the last page goes on from.
  let resumeAt = 0;
  const broken = new Set();

  const cover = el('img', { className: 'player-cover', alt: '', hidden: true });
  const artist = el('b', { className: 'player-artist' });
  const title = el('span', { className: 'player-title' });
  const gone = el('span', { className: 'player-broken', hidden: true, textContent: tr('Трек недоступен') });
  const site = el('span', { className: 'player-site' });
  const embed = el('div', { className: 'player-embed', hidden: true });
  const toggle = iconButton('player-toggle', tr('Слушать'), () => togglePlay());
  const prev = iconButton('player-prev', tr('Предыдущая'), () => step(-1));
  const next = iconButton('player-next', tr('Следующая'), () => step(1));
  const now = el('span', { className: 'player-now', textContent: '0:00' });
  const total = el('span', { className: 'player-total', textContent: '0:00' });
  const seek = el('input', { type: 'range', className: 'player-seek', min: 0, max: 1000, value: 0, ariaLabel: tr('Перемотка') });
  const timeline = el('div', { className: 'player-timeline' }, now, seek, total);
  const shuffleButton = iconButton('player-shuffle', tr('Перемешать'), () => setShuffle(!shuffle));
  const repeatButton = iconButton('player-repeat', tr('Повторять песню'), () => setRepeat(!repeat));
  const muteButton = iconButton('player-mute', tr('Без звука'), () => setMuted(!muted));
  const volume = el('input', { type: 'range', className: 'player-volume', min: 0, max: 100, value: 80, ariaLabel: tr('Громкость') });
  const loud = el('span', { className: 'player-loud' }, muteButton, volume);
  const list = el('div', { className: 'player-queue' });
  const queueBox = el('div', { className: 'player-queue-box', hidden: true },
    el('div', { className: 'player-queue-title', textContent: tr('Список воспроизведения') }), list);
  const queueButton = iconButton('player-list', tr('Список воспроизведения'), () => showQueue(queueBox.hidden));
  const close = el('button', { type: 'button', className: 'player-close', title: tr('Закрыть'), ariaLabel: tr('Закрыть'), textContent: '×', onclick: stop });

  const box = el('div', { className: 'player player-bar', hidden: true },
    embed,
    queueBox,
    el('div', { className: 'player-inner' },
      el('div', { className: 'player-buttons' }, prev, toggle, next),
      cover,
      el('div', { className: 'player-middle' },
        el('div', { className: 'player-names' }, artist, ' — ', title, gone, site),
        timeline),
      el('div', { className: 'player-extra' }, shuffleButton, repeatButton, loud, queueButton),
      close));
  root.append(box);

  volume.addEventListener('input', () => setLoudness(Number(volume.value)));
  seek.addEventListener('input', () => {
    if (media.duration) media.currentTime = (Number(seek.value) / 1000) * media.duration;
  });
  media.addEventListener('timeupdate', () => showTime(media.currentTime, media.duration));
  media.addEventListener('loadedmetadata', () => {
    if (resumeAt > 0 && resumeAt < media.duration) media.currentTime = resumeAt;
    resumeAt = 0;
    showTime(media.currentTime, media.duration);
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
    if (event.key === 'Escape' && !queueBox.hidden) showQueue(false);
  });
  document.addEventListener('click', (event) => {
    if (!queueBox.hidden && !queueBox.contains(event.target) && event.target !== queueButton) showQueue(false);
  });

  const current = () => queue[index] ?? null;

  function showTime(at, length) {
    position = at;
    now.textContent = audioTime(at);
    if (Number.isFinite(length) && length > 0) {
      total.textContent = audioTime(length);
      seek.value = String(Math.round((at / length) * 1000));
    }
  }

  function setOn(value) {
    on = value;
    toggle.classList.toggle('on', on);
    toggle.title = on ? tr('Пауза') : tr('Слушать');
    toggle.ariaLabel = toggle.title;
    box.classList.toggle('playing', on);
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
    if (value > 0 && muted) setMuted(false);
    media.volume = muted ? 0 : value / 100;
  }

  function setMuted(value) {
    muted = value;
    muteButton.classList.toggle('active', muted);
    muteButton.title = muted ? tr('Включить звук') : tr('Без звука');
    muteButton.ariaLabel = muteButton.title;
    media.volume = muted ? 0 : loudness / 100;
  }

  function showQueue(show) {
    queueBox.hidden = !show;
    queueButton.classList.toggle('active', show);
    if (show) list.querySelector('.playing')?.scrollIntoView({ block: 'nearest' });
  }

  // tell is "слушает: …" under the name of the one who listens, once a
  // song; a guest has no name to show it under.
  function tell() {
    const audio = current();
    if (!audio || told === audio.id || !signedIn()) return;

    told = audio.id;
    api.listen(audio.id).catch(() => {});
  }

  // say tells the page what plays.
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
        shuffle, repeat, loudness, muted, broken: [...broken],
      }));
    } catch {
      // A tab that keeps nothing starts with no song.
    }
  }

  function show(audio) {
    artist.textContent = audioArtist(audio);
    title.textContent = audioTitle(audio);
    cover.hidden = !audio.cover;
    if (audio.cover) cover.src = audio.cover;
    gone.hidden = !broken.has(audio.id);
    site.replaceChildren(...(audio.link ? [externalLink(audio.link, [audioSite(audio)])] : []));
    now.textContent = '0:00';
    total.textContent = audioTime(audio.duration);
    seek.value = '0';
    position = 0;

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
    show(audio);
    media.pause();
    frame?.remove();
    frame = null;
    resumeAt = from;

    const file = audio.kind === 'file';
    embed.hidden = file;
    box.classList.toggle('with-embed', !file);
    timeline.classList.toggle('player-off', !file);
    loud.classList.toggle('player-off', !file);
    if (file) {
      media.src = audio.url;
      // A page that opened without a click may not play by itself: then ▶
      // waits for one.
      if (autoplay) media.play().catch(() => setOn(false));
      else setOn(false);
    } else {
      media.removeAttribute('src');
      media.load();
      frame = el('iframe', {
        className: 'player-frame ' + audio.kind,
        src: embedURL(audio, { autoplay, from }),
        allow: 'autoplay; encrypted-media',
        referrerPolicy: 'strict-origin-when-cross-origin',
        title: `${audioArtist(audio)} — ${audioTitle(audio)}`,
      });
      frame.addEventListener('load', () => listenTo(audio));
      embed.append(frame);
      setOn(false);
    }
    say();
  }

  // listenTo asks the player of the site to tell what it does.
  function listenTo(audio) {
    if (audio.kind === 'youtube') {
      post({ event: 'listening', id: 'freenet', channel: 'widget' });
      post({ event: 'command', func: 'addEventListener', args: ['onStateChange'] });
      post({ event: 'command', func: 'addEventListener', args: ['onError'] });
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
      for (const name of ['play', 'pause', 'finish', 'error', 'playProgress']) post({ method: 'addEventListener', value: name });
      if (resumeAt > 0) post({ method: 'seekTo', value: resumeAt * 1000 });
      resumeAt = 0;
    }
    if (method === 'playProgress' && typeof value?.currentPosition === 'number') position = value.currentPosition / 1000;
    if (method === 'play') setOn(true);
    if (method === 'pause') setOn(false);
    if (method === 'finish') ended();
    if (method === 'error') fail();
  }

  function onYouTube({ event, info }) {
    const state = event === 'onStateChange' ? info : event === 'infoDelivery' ? info?.playerState : undefined;
    if (event === 'infoDelivery' && typeof info?.currentTime === 'number') position = info.currentTime;
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
    showQueue(false);
    setOn(false);
    forget();
    if (signedIn()) api.listen('').catch(() => {});
    told = '';
  }

  function renderQueue() {
    list.replaceChildren(...queue.map((a, i) => {
      const row = el('a', { className: 'player-row', onclick: () => start(i) },
        el('span', { className: 'player-row-name' }, el('b', { textContent: audioArtist(a) }), ' — ', el('span', { textContent: audioTitle(a) })),
        el('span', { className: 'player-row-time', textContent: a.duration ? audioTime(a.duration) : audioSite(a) }));
      row.dataset.audio = a.id;
      return row;
    }));
  }

  function play(audio, songs = [audio]) {
    queue = songs.some((a) => a.id === audio.id) ? songs.slice(0, 1000) : [audio];
    renderQueue();
    start(queue.findIndex((a) => a.id === audio.id));
  }

  // restore opens the bar where the last page of the tab left it.
  function restore(state) {
    queue = state.queue;
    for (const id of state.broken ?? []) broken.add(id);
    told = state.told ?? '';
    setShuffle(Boolean(state.shuffle));
    setRepeat(Boolean(state.repeat));
    setLoudness(Number.isFinite(state.loudness) ? state.loudness : 80);
    setMuted(Boolean(state.muted));
    renderQueue();
    start(state.index, { autoplay: Boolean(state.on), from: Number(state.position) || 0 });
  }

  if ('mediaSession' in navigator) {
    navigator.mediaSession.setActionHandler('previoustrack', () => step(-1));
    navigator.mediaSession.setActionHandler('nexttrack', () => step(1));
  }

  return { play, toggle: togglePlay, restore };
}
