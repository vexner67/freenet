import { api, currentUser, started } from './api.js';
import { $, avatarURL, el, fullName, pagePath, renderChrome, verifiedMark, autosize } from './layout.js';
import { audioTitle, errorText, formatPostDate } from './labels.js';
import { pickRoomAudio } from './audio.js';
import { initStickers, stickerNode } from './stickers.js';
import { roomRuntime } from './room-runtime.js';
import { roomKind } from './room-list.js';
import { roomYouTubeID, playbackPosition } from './room-sync.js';
import { inCall } from './calls.js';
import { tr } from './i18n.js';

const id = location.pathname.split('/').filter(Boolean)[1] ?? '';
const wanted = started(api.room(id));
const me = await currentUser().catch(() => null);
renderChrome(me);
document.documentElement.classList.add('room-page');
const runtime = await roomRuntime();
let view = null;
let live = JSON.parse(runtime.snapshot());
let memberKey = '';
let chatKey = '';
let editingScreen = false;
const silenced = new Set();
const active = () => live.joined && live.room?.id === id;
const current = () => active() ? live.room : view;
const showError = (e) => { $('error').textContent = typeof e === 'string' ? e : errorText(e); $('error').hidden = false; };
async function run(fn) {
  $('error').hidden = true;
  try { await fn(); } catch (e) { showError(e); }
}

function dock() {
  if (!active() || (window.parent === window && document.documentElement.classList.contains('shell'))) return;
  const room = current();
  const rect = room?.kind === 'watch' && room.playback.kind === 'youtube' ? $('room-video-slot').getBoundingClientRect() : null;
  runtime.dockScreen(rect ? { left: rect.left, top: rect.top, width: rect.width, height: rect.height } : null, window.parent !== window);
}
function members(room) {
  const key = JSON.stringify([room.members, room.ownerId, active(), room.maxMembers]);
  if (key !== memberKey) {
    memberKey = key;
    $('room-members').style.setProperty('--room-rows', String(Math.ceil(room.maxMembers / 5)));
    $('room-members').replaceChildren(...Array.from({ length: room.maxMembers }, (_, index) => {
      const member = room.members[index];
      if (!member) return el('div', { className: 'room-member empty', ariaHidden: 'true' });
      const user = member.user;
      const tile = el('div', { className: 'room-member' });
      tile.dataset.user = user.id;
      tile.append(el('a', { href: pagePath(user) }, el('img', { src: avatarURL(user, 'small'), width: 64, height: 64, alt: fullName(user) })),
        el('div', { className: 'room-member-name' }, el('a', { href: pagePath(user), textContent: fullName(user) }), verifiedMark(user)),
        el('div', { className: 'hint room-member-state' }));
      if (active() && user.id !== me?.id) {
        if (runtime.blocked(user.id)) tile.append(el('div', { className: 'hint', textContent: tr('Звук недоступен') }));
        else {
          const silence = el('a', { textContent: silenced.has(user.id) ? tr('Слышать') : tr('Не слышать') });
          silence.addEventListener('click', () => {
            if (silenced.has(user.id)) silenced.delete(user.id); else silenced.add(user.id);
            runtime.silence(user.id, silenced.has(user.id));
            silence.textContent = silenced.has(user.id) ? tr('Слышать') : tr('Не слышать');
          });
          tile.append(silence);
        }
        if (room.ownerId === me?.id) tile.append(el('div', { className: 'room-member-tools' },
          el('a', { textContent: tr('Выключить микрофон'), onclick: () => run(() => api.roomMember(id, 'mute', user.id)) }),
          el('a', { textContent: tr('Выгнать'), onclick: () => run(() => api.roomMember(id, 'kick', user.id)) })));
      }
      return tile;
    }));
  }
  for (const tile of $('room-members').querySelectorAll('[data-user]')) {
    const speaking = active() && live.speaking.includes(tile.dataset.user);
    tile.classList.toggle('speaking', speaking);
    tile.querySelector('.room-member-state').textContent = speaking ? tr('говорит') : tile.dataset.user === me?.id && live.muted && active() ? tr('микрофон выключен') : '';
  }
}
function renderChat() {
  const messages = active() ? live.chat : [];
  const key = messages.map((message) => message.id).join(',');
  if (key === chatKey) return;
  chatKey = key;
  const list = $('room-chat');
  const bottom = list.scrollHeight - list.scrollTop - list.clientHeight < 50;
  const old = list.scrollTop;
  list.replaceChildren(...messages.map((message) => el('div', { className: 'room-chat-message' },
    el('a', { href: pagePath(message.author), textContent: fullName(message.author) }), verifiedMark(message.author),
    el('span', { className: 'hint', textContent: ` · ${formatPostDate(message.createdAt)}` }),
    message.sticker ? stickerNode(message.sticker) : el('div', { className: 'room-chat-text', textContent: message.text }))));
  if (bottom) list.scrollTop = list.scrollHeight; else list.scrollTop = old;
}
function render() {
  const room = current();
  if (!room) return;
  document.title = `${room.title} | Freenet`;
  $('title').textContent = room.title;
  $('room-description').textContent = tr('{0} · {1} из {2}', roomKind(room.kind), room.members.length, room.maxMembers);
  $('room-join').hidden = active() || !me;
  $('room-join').disabled = live.joining;
  $('room-guest').hidden = Boolean(me);
  $('room-own-controls').hidden = !active();
  $('room-microphone').textContent = live.muted ? tr('Включить микрофон') : tr('Выключить микрофон');
  $('room-invite').hidden = !active();
  $('room-settings').hidden = room.ownerId !== me?.id;
  $('room-close').hidden = room.ownerId !== me?.id;
  $('room-chat-form').hidden = !active();
  $('room-chat-empty').hidden = active() && live.chat.length > 0;
  $('room-screen-panel').hidden = room.kind === 'talk' && !room.playback.kind && !editingScreen;
  $('room-open-screen').hidden = !active() || (room.control !== 'everyone' && room.ownerId !== me?.id) || room.kind !== 'talk';
  $('room-video-slot').hidden = room.kind !== 'watch';
  $('room-screen-panel').classList.toggle('listen', room.kind === 'listen');
  $('room-screen-controls').hidden = !active() || (room.control !== 'everyone' && room.ownerId !== me?.id);
  $('room-screen-toggle').textContent = room.playback.playing ? tr('Пауза') : tr('Воспроизвести');
  $('room-screen-toggle').disabled = !room.playback.kind;
  $('room-screen-next').disabled = !room.playback.queue.length;
  $('room-screen-time').textContent = room.playback.kind ? tr('Позиция: {0} с', Math.floor(playbackPosition(room.playback, Date.now()))) : tr('Общий экран пока пуст.');
  $('room-queue').replaceChildren(...room.playback.queue.map((item) => el('li', { textContent: item.title || item.mediaId })));
  const statuses = { connecting: tr('Соединяемся…'), connected: tr('Голос подключён'), reconnecting: tr('Переподключаемся…'), disconnected: tr('Соединение с голосом прервано.'), microphone_denied: tr('Микрофон недоступен. Вы можете слушать комнату.'), audio_blocked: tr('Разрешите воспроизведение звука.') };
  $('room-status').textContent = active() ? statuses[live.status] ?? '' : '';
  $('room-enable-audio').hidden = !active() || live.status !== 'audio_blocked';
  if (active() && live.error) showError(live.error);
  members(room);
  renderChat();
  dock();
}

$('room-join').addEventListener('click', () => run(async () => {
  if (inCall()) { showError(tr('Сначала завершите звонок.')); return; }
  await runtime.join(id, me.id);
}));
$('room-microphone').addEventListener('click', () => run(() => runtime.mute(!live.muted)));
$('room-leave').addEventListener('click', () => run(() => runtime.leave()));
$('room-enable-audio').addEventListener('click', () => run(() => runtime.startAudio()));
$('room-copy').addEventListener('click', () => run(async () => {
  const link = `${location.origin}/room/${encodeURIComponent(id)}`;
  try { await navigator.clipboard.writeText(link); $('room-status').textContent = tr('Ссылка скопирована.'); }
  catch { window.prompt(tr('Ссылка на комнату'), link); }
}));
$('room-help').addEventListener('click', () => { $('room-help-text').hidden = !$('room-help-text').hidden; });
$('room-settings').addEventListener('click', () => {
  $('room-settings-form').hidden = !$('room-settings-form').hidden;
  if ($('room-settings-form').hidden) return;
  const room = current();
  $('room-edit-title').value = room.title; $('room-edit-kind').value = room.kind;
  if (room.clubId && !$('room-edit-access').querySelector('option[value="club"]')) $('room-edit-access').append(el('option', { value: 'club', textContent: tr('участники сообщества') }));
  $('room-edit-access').value = room.access; $('room-edit-control').value = room.control;
});
$('room-settings-form').addEventListener('submit', (event) => {
  event.preventDefault();
  run(async () => {
    const result = await api.updateRoom(id, { title: $('room-edit-title').value, kind: $('room-edit-kind').value, access: $('room-edit-access').value, control: $('room-edit-control').value });
    view = result.room; $('room-settings-form').hidden = true;
    if (!active()) render();
  });
});
$('room-close').addEventListener('click', () => run(async () => {
  if (!window.confirm(tr('Закрыть комнату для всех участников?'))) return;
  await api.closeRoom(id);
  if (active()) await runtime.leave();
  location.assign('/rooms');
}));
$('room-invite').addEventListener('click', () => { $('room-invite-form').hidden = !$('room-invite-form').hidden; });
$('room-invite-form').addEventListener('submit', (event) => {
  event.preventDefault(); run(async () => {
    const user = await api.profile($('room-invite-name').value.trim());
    await api.roomMember(id, 'invite', user.id);
    $('room-invite-name').value = '';
    $('room-status').textContent = tr('Приглашение отправлено.');
  });
});
$('room-chat-form').addEventListener('submit', (event) => {
  event.preventDefault();
  const text = $('room-chat-text').value.trim();
  if (!text) return;
  run(async () => {
    $('room-chat-send').disabled = true;
    try { await runtime.sendChat(text); $('room-chat-text').value = ''; autosize($('room-chat-text')); }
    finally { $('room-chat-send').disabled = false; }
  });
});
const showStickers = initStickers({ panel: $('room-stickers'), error: $('error'), onPick: async (sticker) => {
  try { await runtime.sendChat('', sticker); return true; } catch (e) { showError(e); return false; }
}, onSent: () => { $('room-stickers').hidden = true; } });
$('room-show-stickers').addEventListener('click', () => { $('room-stickers').hidden = !$('room-stickers').hidden; if (!$('room-stickers').hidden) showStickers(); });
$('room-youtube-form').addEventListener('submit', (event) => {
  event.preventDefault(); run(async () => {
    const mediaId = roomYouTubeID($('room-youtube').value);
    if (!mediaId) { showError(tr('Вставьте ссылку на видео YouTube.')); return; }
    await runtime.command('load', { kind: 'youtube', mediaId });
    $('room-youtube').value = '';
  });
});
$('room-add-audio').addEventListener('click', () => pickRoomAudio((audio) => run(() => runtime.command('enqueue', { kind: 'audio', mediaId: audio.id, title: audioTitle(audio) }))));
$('room-open-screen').addEventListener('click', (event) => { event.preventDefault(); editingScreen = !editingScreen; render(); });
$('room-screen-toggle').addEventListener('click', () => run(() => runtime.command('toggle')));
$('room-screen-next').addEventListener('click', () => run(() => runtime.command('next')));
$('room-seek-form').addEventListener('submit', (event) => { event.preventDefault(); run(() => runtime.command('seek', Number($('room-seek').value))); });
const cancel = runtime.onChange((raw) => { live = JSON.parse(raw); render(); });
try { view = (await wanted).room; render(); } catch (e) { showError(e); }
const poll = setInterval(() => { if (!document.hidden && !active()) api.room(id).then((result) => { view = result.room; render(); }).catch(showError); }, 15_000);
const resize = new ResizeObserver(dock); resize.observe($('room-video-slot'));
window.addEventListener('scroll', dock, { passive: true });
window.addEventListener('resize', dock);
window.addEventListener('pagehide', () => { cancel(); clearInterval(poll); resize.disconnect(); runtime.dockScreen(null); }, { once: true });
