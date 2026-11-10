// The shared screen uses the site's player. Its media element stays in the
// outer document, while pages provide only a rectangle and server commands.
import { api } from './api.js';
import { el } from './layout.js';
import { createPlayer } from './player.js';
import { needsRoomSeek, playbackPosition } from './room-sync.js';
import { tr } from './i18n.js';

export function createRoomScreen({ onEnded, onError }) {
  const host = el('div', { className: 'room-media-host room-media-away' });
  document.body.append(host);
  let playback = null;
  let clockOffset = 0;
  let mediaKey = '';
  let actual = { position: 0, length: 0, on: false };
  let failed = '';
  const player = createPlayer(host, {
    embedded: true,
    onEnded(audio) {
      if (audio?.id === playback?.mediaId && playback.playing) onEnded(playback.version);
    },
    onState(state) {
      actual = state;
      if (state.id && state.broken.includes(state.id) && failed !== state.id) {
        failed = state.id;
        onError(tr('Этот трек или видео нельзя воспроизвести на Freenet.'));
      }
    },
  });
  const sync = () => {
    if (!playback?.kind || failed === actual.id) return;
    const position = playbackPosition(playback, Date.now() + clockOffset);
    if (actual.id === playback.mediaId && actual.length > 0 && actual.position >= actual.length - 0.05 && position >= actual.length - 2 && playback.playing) {
      onEnded(playback.version);
      return;
    }
    if (needsRoomSeek(actual.position, position)) player.position(position);
    player.setPlaying(playback.playing);
  };
  const timer = setInterval(sync, 1000);

  async function apply(next, offset) {
    playback = next;
    clockOffset = offset;
    const key = `${next.kind}:${next.mediaId}`;
    if (key === mediaKey) { sync(); return; }
    mediaKey = key;
    failed = '';
    if (!next.kind) { player.stop(); return; }
    try {
      const audio = next.kind === 'youtube'
        ? { id: next.mediaId, kind: 'youtube', link: `https://www.youtube.com/watch?v=${encodeURIComponent(next.mediaId)}`, artist: 'YouTube', title: tr('Общий экран') }
        : (await api.audioByID(next.mediaId)).audio;
      if (key !== mediaKey) return;
      player.restore({ queue: [audio], index: 0, on: playback.playing,
        position: playbackPosition(playback, Date.now() + clockOffset), loudness: 80, muted: false });
    } catch (error) { if (key === mediaKey) onError(error); }
  }
  return {
    apply,
    dock(rect, framed = false) {
      if (!rect || !playback?.kind || playback.kind !== 'youtube') { host.classList.add('room-media-away'); return; }
      const offset = framed ? document.querySelector('.shell-frame')?.getBoundingClientRect() : null;
      if (![rect.left, rect.top, rect.width, rect.height].every(Number.isFinite)) return;
      host.classList.remove('room-media-away');
      host.style.left = `${rect.left + (offset?.left ?? 0)}px`;
      host.style.top = `${rect.top + (offset?.top ?? 0)}px`;
      host.style.width = `${Math.max(0, rect.width)}px`;
      host.style.height = `${Math.max(0, rect.height)}px`;
    },
    mute(value) { player.mute(value); },
    close() { clearInterval(timer); mediaKey = ''; playback = null; player.destroy(); host.remove(); },
  };
}
