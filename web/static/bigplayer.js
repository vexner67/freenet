// openvk's big player (bigplayer.latte, al_music.js) at the top of
// "Аудиозаписи": ▶ with ◀◀ and ▶▶ under it, "Исполнитель — Название" with
// "00:00 / -03:05" over the line of the song, the line of the loudness,
// "Повторение", "Перемешать" and "Заглушить". It shows and steers the
// player of the tab (player.js through audio.js), whose box hides
// meanwhile; with nothing played yet ▶ plays the songs of the page from
// the first. Its keys are openvk's (track.js) while no field is typed in.

import { audioArtist, audioTime, audioTitle, audioSite } from './labels.js';
import { el } from './layout.js';
import { control, dockPlayer, listen, play, trackLine } from './audio.js';
import { shortcut } from './track.js';
import { tr } from './i18n.js';

function sign(className, label, onclick) {
  return el('button', { type: 'button', className: 'bp-sign ' + className, title: label, ariaLabel: label, onclick });
}

// bigPlayer fills box; songs gives the songs of the page, in their order.
export function bigPlayer(box, songs) {
  let state = null;
  // left: the time still to go after the slash, else the whole length.
  let left = true;

  const start = () => {
    const list = songs();
    if (list.length) play(list[0], list);
  };
  const toggle = () => (state?.audio ? control('toggle') : start());

  const playSign = sign('bp-play', tr('Проигрывание/пауза'), toggle);
  const artist = el('b', { className: 'bp-artist' });
  const title = el('span', { className: 'bp-title' });
  const gone = el('span', { className: 'bp-broken', hidden: true, textContent: tr('Трек недоступен') });
  const name = el('a', { className: 'bp-name' }, artist, ' — ', title, gone);
  const time = el('span', { className: 'bp-time', textContent: '00:00' });
  const rest = el('span', {
    className: 'bp-rest', textContent: '-00:00',
    onclick: () => {
      left = !left;
      if (state) show(state);
    },
  });
  const seek = trackLine({ label: tr('Перемотка'), onPick: (part) => control('seek', part), tip: (part) => audioTime(part * (state?.length ?? 0)) });
  const volume = trackLine({ label: tr('Громкость'), onPick: (part) => control('volume', part * 100), tip: (part) => `${Math.round(part * 100)}%` });
  const repeat = sign('bp-repeat', tr('Повторение'), () => control('repeat', !state?.repeat));
  const shuffle = sign('bp-shuffle', tr('Перемешать'), () => control('shuffle', !state?.shuffle));
  const mute = sign('bp-mute', tr('Заглушить'), () => control('mute', !state?.muted));

  box.replaceChildren(el('div', { className: 'bp-wrap' },
    el('div', { className: 'bp-buttons' },
      playSign,
      el('div', { className: 'bp-arrows' },
        sign('bp-prev', tr('Предыдущая'), () => control('prev')),
        sign('bp-next', tr('Следующая'), () => control('next')))),
    el('div', { className: 'bp-track' },
      el('div', { className: 'bp-info' }, name, el('span', { className: 'bp-timer' }, time, ' / ', rest)),
      seek.node),
    el('div', { className: 'bp-volume' }, volume.node),
    el('div', { className: 'bp-extra' }, repeat, shuffle, mute)));

  function show(next) {
    state = next;
    const audio = next.audio;
    artist.textContent = audioArtist(audio ?? {});
    title.textContent = audioTitle(audio ?? {});
    gone.hidden = !audio || !next.broken?.includes(audio.id);
    // A track names its site: the name opens it there.
    if (audio?.link) {
      name.href = audio.link;
      name.target = '_blank';
      name.rel = 'noopener noreferrer';
      name.title = tr('Слушать на {0}', audioSite(audio));
    } else {
      name.removeAttribute('href');
      name.removeAttribute('title');
    }
    const length = next.length || audio?.duration || 0;
    time.textContent = audioTime(next.position);
    rest.textContent = left ? '-' + audioTime(Math.max(0, length - next.position)) : audioTime(length);
    seek.set(length ? Math.min(1, next.position / length) : 0);
    volume.set(next.loudness / 100);
    playSign.classList.toggle('on', Boolean(next.on));
    for (const [node, on] of [[repeat, next.repeat], [shuffle, next.shuffle], [mute, next.muted]]) {
      node.classList.toggle('pressed', Boolean(on));
      node.ariaPressed = String(Boolean(on));
    }
  }

  document.addEventListener('keydown', (event) => {
    const what = shortcut(event.code);
    if (!what || event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.target.closest?.('input, textarea, select, [contenteditable], button, a') && event.code === 'Space') return;
    if (event.target.closest?.('input, textarea, select, [contenteditable]') || document.querySelector('.modal, .sheet')) return;

    event.preventDefault();
    const [name, value] = what;
    if (name === 'toggle') toggle();
    else if (name === 'louder') control('volume', (state?.loudness ?? 80) + value);
    else if (name === 'repeat' || name === 'mute' || name === 'shuffle') control(name, !state?.[name === 'mute' ? 'muted' : name]);
    else control(name, value ?? null);
  });

  dockPlayer();
  listen(show);
}
