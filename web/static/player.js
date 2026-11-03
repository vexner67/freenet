// The player of songs: the song that plays with its cover and names, the
// buttons of a player of the 2000s and the list it plays. An uploaded MP3
// plays in an <audio> of its own; a track of SoundCloud or YouTube plays
// in the player of its site, shown as it is and told what to do by
// postMessage, as their widget APIs have it — no scripts of theirs run
// here. A song that does not play is grey, "Трек недоступен", and the
// next one plays.
//
// On a computer it fills the window /player (player-page.js); on a phone
// it is a bar over the bottom of the page (bar: true). It says what plays
// to every page over the BroadcastChannel of audio.js.

import { api, signedIn } from './api.js';
import { CHANNEL } from './audio.js';
import { audioArtist, audioSite, audioTime, audioTitle } from './labels.js';
import { el, externalLink } from './layout.js';
import { tr } from './i18n.js';

const SOUNDCLOUD = 'https://w.soundcloud.com';
const YOUTUBE = 'https://www.youtube-nocookie.com';
// How long "Трек недоступен" shows before the next song plays.
const SKIP_AFTER = 1500;

// youTubeID is the video of a link the server made canonical
// ("https://www.youtube.com/watch?v=<id>").
function youTubeID(link) {
  try {
    return new URL(link).searchParams.get('v') ?? '';
  } catch {
    return '';
  }
}

function embedURL(audio) {
  if (audio.kind === 'soundcloud') {
    const query = new URLSearchParams({
      url: audio.link, auto_play: 'true', hide_related: 'true', show_comments: 'false',
      show_reposts: 'false', show_teaser: 'false', visual: 'false', color: '#595959',
    });
    return `${SOUNDCLOUD}/player/?${query}`;
  }

  const query = new URLSearchParams({ autoplay: '1', enablejsapi: '1', playsinline: '1', rel: '0', origin: location.origin });
  return `${YOUTUBE}/embed/${encodeURIComponent(youTubeID(audio.link))}?${query}`;
}

// createPlayer puts a player into root and returns what plays songs.
export function createPlayer(root, { bar = false } = {}) {
  const channel = typeof BroadcastChannel === 'function' ? new BroadcastChannel(CHANNEL) : null;
  const media = new Audio();
  media.preload = 'auto';

  let queue = [];
  let index = -1;
  let on = false;
  let frame = null;
  let told = '';
  const broken = new Set();

  const cover = el('img', { className: 'player-cover', alt: '', hidden: true });
  const artist = el('b', { className: 'player-artist' });
  const title = el('span', { className: 'player-title' });
  const gone = el('div', { className: 'player-broken', hidden: true, textContent: tr('Трек недоступен') });
  const site = el('div', { className: 'player-site' });
  const embed = el('div', { className: 'player-embed', hidden: true });
  const toggle = el('button', { type: 'button', className: 'player-toggle', title: tr('Слушать'), ariaLabel: tr('Слушать'), onclick: () => togglePlay() });
  const prev = el('button', { type: 'button', className: 'player-prev', title: tr('Предыдущая'), ariaLabel: tr('Предыдущая'), onclick: () => step(-1) });
  const next = el('button', { type: 'button', className: 'player-next', title: tr('Следующая'), ariaLabel: tr('Следующая'), onclick: () => step(1) });
  const now = el('span', { className: 'player-now', textContent: '0:00' });
  const total = el('span', { className: 'player-total', textContent: '0:00' });
  const seek = el('input', { type: 'range', className: 'player-seek', min: 0, max: 1000, value: 0, ariaLabel: tr('Перемотка') });
  const volume = el('input', { type: 'range', className: 'player-volume', min: 0, max: 100, value: 80, ariaLabel: tr('Громкость') });
  const timeline = el('div', { className: 'player-timeline' }, now, seek, total);
  const list = el('div', { className: 'player-queue' });
  const close = bar ? el('button', { type: 'button', className: 'player-close', title: tr('Закрыть'), ariaLabel: tr('Закрыть'), textContent: '×', onclick: stop }) : null;

  const box = el('div', { className: bar ? 'player player-bar' : 'player player-full', hidden: bar },
    el('div', { className: 'player-head' },
      cover,
      el('div', { className: 'player-names' }, artist, title, gone, site),
      close),
    embed,
    el('div', { className: 'player-controls' }, prev, toggle, next, timeline, bar ? null : volume),
    bar ? null : el('div', { className: 'player-queue-title', textContent: tr('Список воспроизведения') }),
    bar ? null : list);
  root.append(box);

  media.volume = 0.8;
  volume.addEventListener('input', () => {
    media.volume = Number(volume.value) / 100;
  });
  seek.addEventListener('input', () => {
    if (media.duration) media.currentTime = (Number(seek.value) / 1000) * media.duration;
  });
  media.addEventListener('timeupdate', () => {
    now.textContent = audioTime(media.currentTime);
    if (media.duration) seek.value = String(Math.round((media.currentTime / media.duration) * 1000));
  });
  media.addEventListener('loadedmetadata', () => {
    if (Number.isFinite(media.duration)) total.textContent = audioTime(media.duration);
  });
  media.addEventListener('playing', () => setOn(true));
  media.addEventListener('pause', () => setOn(false));
  media.addEventListener('ended', () => step(1, true));
  media.addEventListener('error', () => {
    if (current()?.kind === 'file' && media.getAttribute('src')) fail();
  });

  window.addEventListener('message', onFrameMessage);
  // The bar of a phone plays what its own page asks (audio.js calls it):
  // other pages only learn what it plays.
  channel?.addEventListener('message', ({ data }) => {
    if (!bar || data?.type === 'hello') command(data);
  });

  const current = () => queue[index] ?? null;

  function setOn(value) {
    on = value;
    toggle.classList.toggle('on', on);
    toggle.title = on ? tr('Пауза') : tr('Слушать');
    toggle.ariaLabel = toggle.title;
    if (on) tell();
    say();
  }

  // tell is "слушает: …" under the name of the one who listens, once a
  // song; a guest has no name to show it under.
  function tell() {
    const audio = current();
    if (!audio || told === audio.id || !signedIn()) return;

    told = audio.id;
    api.listen(audio.id).catch(() => {});
  }

  // say tells every page what plays.
  function say() {
    channel?.postMessage({ type: 'state', id: current()?.id ?? '', on, broken: [...broken] });
  }

  function show(audio) {
    artist.textContent = audioArtist(audio);
    title.textContent = audioTitle(audio);
    cover.hidden = !audio.cover;
    if (audio.cover) cover.src = audio.cover;
    gone.hidden = !broken.has(audio.id);
    site.replaceChildren(...(audio.link ? [externalLink(audio.link, [tr('Слушать на {0}', audioSite(audio))])] : []));
    now.textContent = '0:00';
    total.textContent = audioTime(audio.duration);
    seek.value = '0';
    // The window of the player is named by its song; a page keeps its name.
    if (!bar) document.title = `${audioArtist(audio)} — ${audioTitle(audio)}`;

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

  function start(i) {
    index = i;
    const audio = current();
    if (!audio) return;

    box.hidden = false;
    told = '';
    show(audio);
    media.pause();
    frame?.remove();
    frame = null;

    const file = audio.kind === 'file';
    embed.hidden = file;
    timeline.hidden = !file;
    // The player of a site has its own loudness.
    volume.hidden = !file;
    if (file) {
      media.src = audio.url;
      // A window that opened without a click may not play by itself:
      // then ▶ waits for one.
      media.play().catch(() => setOn(false));
    } else {
      media.removeAttribute('src');
      media.load();
      frame = el('iframe', {
        className: 'player-frame ' + audio.kind,
        src: embedURL(audio),
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

  function onSoundCloud({ method }) {
    if (method === 'ready') {
      for (const name of ['play', 'pause', 'finish', 'error']) post({ method: 'addEventListener', value: name });
    }
    if (method === 'play') setOn(true);
    if (method === 'pause') setOn(false);
    if (method === 'finish') step(1, true);
    if (method === 'error') fail();
  }

  function onYouTube({ event, info }) {
    const state = event === 'onStateChange' ? info : event === 'infoDelivery' ? info?.playerState : undefined;
    if (state === 1) setOn(true);
    if (state === 2) setOn(false);
    if (state === 0) step(1, true);
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

  // step plays the song by, skipping those that do not play; at the end
  // of the list (ended: the last one is over) it stops.
  function step(by, ended = false) {
    for (let i = index + by; i >= 0 && i < queue.length; i += by) {
      if (!broken.has(queue[i].id)) {
        start(i);
        return;
      }
    }
    if (ended) {
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
    frame?.remove();
    frame = null;
    index = -1;
    queue = [];
    box.hidden = true;
    setOn(false);
    if (signedIn()) api.listen('').catch(() => {});
  }

  function renderQueue() {
    list.replaceChildren(...queue.map((a, i) => {
      const row = el('a', { className: 'player-row', onclick: () => start(i) },
        el('b', { textContent: audioArtist(a) }), ' — ', el('span', { textContent: audioTitle(a) }),
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

  // command is what a page asks over the channel.
  function command(data) {
    if (data?.type === 'hello') say();
    if (data?.type === 'toggle') togglePlay();
    if (data?.type === 'play' && data.audio) {
      channel?.postMessage({ type: 'got' });
      play(data.audio, data.queue);
    }
  }

  if ('mediaSession' in navigator) {
    navigator.mediaSession.setActionHandler('previoustrack', () => step(-1));
    navigator.mediaSession.setActionHandler('nexttrack', () => step(1));
  }

  return { play, command, ready: () => channel?.postMessage({ type: 'ready' }) };
}
