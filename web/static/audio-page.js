// "Аудиозаписи" (/audio, /audio/<nickname>, /playlist/<id>), as openvk's
// Audio/List.latte: the big player at the top (bigplayer.js), the songs
// under it at the left and openvk's grey links at the right — "Моя
// музыка" (the songs one keeps, the last added first), "Загрузить аудио",
// "Добавить по ссылке", "Новое" (every song of the site, the newest
// first), "Мои плейлисты", "Новый плейлист", and on someone else's page
// "Музыка пользователя" and "Плейлисты пользователя". A search of all songs
// is over the list. A playlist is openvk's Playlist.latte: its cover with
// links under it at the left, its name and songs at the right.

import { api, currentUser, started } from './api.js';
import { audioSite, audiosCount, formatPostDate } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { openModal, button, toast } from './modal.js';
import { addByLink, audioNode, confirmDelete, editAudio, forget, keep, keepIcon, nameWindow, newPlaylist, pickPlaylist, uploadAudio } from './audio.js';
import { bigPlayer } from './bigplayer.js';
import { tr } from './i18n.js';

const [kind, ref = ''] = location.pathname.split('/').filter(Boolean);
const section = new URLSearchParams(location.search).get('section') ?? '';
const playlistID = kind === 'playlist' ? ref : '';
const nickname = kind === 'audio' ? decodeURIComponent(ref) : '';

// The songs shown, in their order: the list a song plays on in.
let shown = [];
let cursor = '';
let total = 0;
let query = '';
let asked = 0;
let me = null;
let owner = null;
let playlist = null;

// first is the main read, sent with /me where it does not need it.
function firstRead() {
  if (playlistID) return api.playlist(playlistID);
  if (section === 'new') return api.searchAudios('');
  if (!nickname) return null;

  return section === 'playlists' ? api.playlists(nickname) : api.audios(nickname);
}

const first = firstRead();
if (first) started(first);
const ownerRead = nickname ? started(api.profile(nickname)) : null;

bigPlayer($('big-player'), () => shown);

const mine = () => Boolean(me && owner && owner.id === me.id);
const ownPlaylist = () => Boolean(me && playlist && playlist.ownerId === me.id);
const showsPlaylists = () => section === 'playlists' && !query && !playlistID;

// emptyText is openvk's content_error of what the page shows.
function emptyText() {
  if (query) return tr('Результатов нет');
  if (playlistID) return tr('В этом плейлисте нет аудиозаписей.');
  if (section === 'new') return tr('Здесь пока нет аудиозаписей.');
  if (section === 'playlists') return mine() ? tr('Вы ещё не добавляли плейлистов.') : tr('Этот пользователь ещё не добавлял плейлистов.');

  return mine() ? tr('Вы ещё не добавляли аудиозаписей.') : tr('Этот пользователь ещё не добавлял аудиозаписей.');
}

function showEmpty() {
  const list = showsPlaylists() ? $('playlists') : $('audios');
  $('empty').hidden = list.childElementCount !== 0;
  $('empty-text').textContent = emptyText();
  if (playlist) $('playlist-meta').textContent = audiosCount(total);
}

// inLibrary tells whether the page is one's own music, where a song one
// takes away goes from the list.
const inLibrary = () => mine() && !playlist && !query && section === '';

// items is "⋯" of a song: what this reader may do with it here.
function items(audio, row) {
  const list = [];
  if (me) {
    list.push(audio.added
      ? { text: tr('Удалить из моих аудиозаписей'), run: () => forget(audio).then(() => untaken(audio, row)) }
      : { text: tr('Добавить к себе'), run: () => keep(audio).then(() => replace(audio, row, audio)) });
    list.push({ text: tr('Добавить в плейлист'), run: () => pickPlaylist(audio) });
    if (ownPlaylist()) list.push({ text: tr('Убрать из плейлиста'), run: () => removeFromPlaylist(audio, row) });
    list.push({ text: tr('Сделать песней страницы'), run: () => setPageAudio(audio) });
    if (audio.kind === 'file' && audio.ownerId === me.id) {
      list.push({ text: tr('Редактировать'), run: () => edit(audio, row) });
      list.push({ text: tr('Удалить'), danger: true, run: () => confirmDelete(audio, () => gone(audio, row)) });
    }
  }
  if (audio.link) list.push({ text: tr('Слушать на {0}', audioSite(audio)), run: () => window.open(audio.link, '_blank', 'noopener') });

  return list;
}

// rowOf is a song of the list; under the pointer, as openvk's, "+" or
// "×" of one's music, the pencil of one's upload, and "⋯" for the rest.
function rowOf(audio) {
  const icons = [];
  if (me) icons.push(keepIcon(audio, { onForget: () => untaken(audio, row) }));
  if (me && audio.kind === 'file' && audio.ownerId === me.id) {
    icons.push(el('button', { type: 'button', className: 'audio-icon audio-icon-edit', title: tr('Редактировать'), ariaLabel: tr('Редактировать'), onclick: () => edit(audio, row) }));
  }
  const row = audioNode(audio, { queue: () => shown, icons, menu: () => items(audio, row) });

  return row;
}

function edit(audio, row) {
  editAudio(audio, (edited) => replace(audio, row, { ...audio, ...edited }));
}

function replace(audio, row, edited) {
  shown[shown.indexOf(audio)] = edited;
  row.replaceWith(rowOf(edited));
}

function gone(audio, row) {
  shown = shown.filter((a) => a !== audio);
  row.remove();
  total--;
  showEmpty();
}

// untaken is a song taken from one's music: it leaves one's library.
function untaken(audio, row) {
  if (audio.added) return;
  if (inLibrary()) gone(audio, row);
  else replace(audio, row, audio);
}

async function removeFromPlaylist(audio, row) {
  try {
    await api.removeFromPlaylist(playlist.id, audio.id);
    gone(audio, row);
  } catch (e) {
    showError($('error'), e);
  }
}

async function setPageAudio(audio) {
  try {
    await api.setPageAudio(audio.id);
    toast(tr('Теперь это песня вашей страницы.'));
  } catch (e) {
    showError($('error'), e);
  }
}

// added puts a song one has just uploaded or added at the top of one's
// library, when it is shown.
function added(audio) {
  toast(tr('Аудиозапись добавлена к вам.'));
  if (!inLibrary()) return;

  shown.unshift(audio);
  $('audios').prepend(rowOf(audio));
  total++;
  showEmpty();
}

// addedToPlaylist puts a song uploaded from the page of a playlist into it.
async function addedToPlaylist(audio) {
  try {
    await api.addToPlaylist(playlist.id, audio.id);
    shown.unshift(audio);
    $('audios').prepend(rowOf(audio));
    total++;
    showEmpty();
  } catch (e) {
    showError($('error'), e);
  }
}

function showSongs(page, fresh) {
  if (fresh) {
    shown = [];
    $('audios').replaceChildren();
  }
  shown.push(...page.audios);
  $('audios').append(...page.audios.map(rowOf));
  cursor = page.nextCursor;
  total = page.total ?? total;
  $('more').hidden = !cursor;
}

// showPlaylists lays out openvk's playlistListView: the cover, the name and
// when it was made.
function showPlaylists(page) {
  $('playlists').append(...page.playlists.map((p) => el('a', { className: 'playlist-view', href: '/playlist/' + encodeURIComponent(p.id) },
    el('span', { className: 'playlist-view-cover' }, el('img', { src: '/static/song.jpg', alt: tr('Обложка плейлиста') })),
    el('span', { className: 'playlist-view-info' },
      el('span', { className: 'playlist-view-name', textContent: p.title }),
      el('span', { className: 'playlist-view-meta', textContent: formatPostDate(p.createdAt) })))));
  cursor = page.nextCursor;
  total = page.total;
  $('more').hidden = !cursor;
}

// read is the next page of what the page shows; fresh starts it over.
async function read(fresh, given = null) {
  const ask = ++asked;
  $('more').hidden = true;
  if (fresh) cursor = '';
  hideError($('error'));

  try {
    let page;
    if (given) page = await given;
    else if (query) page = await api.searchAudios(query, cursor);
    else if (playlistID) page = await api.playlist(playlistID, cursor);
    else if (section === 'new') page = await api.searchAudios('', cursor);
    else if (section === 'playlists') page = await api.playlists(owner.nickname, cursor);
    else page = await api.audios(owner.nickname, cursor);
    if (ask !== asked) return;

    if (showsPlaylists()) showPlaylists(page);
    else showSongs(page, fresh);
    showEmpty();
  } catch (e) {
    showError($('error'), e);
  }
}

// heading is openvk's header of the page: "Мои аудиозаписи", "Имя »
// Аудиозаписи", "Аудиозаписи » Новое", "Аудиозаписи » Мои плейлисты",
// "Имя » Плейлисты » Плейлист".
function heading() {
  const ownerLink = (who) => el('a', { href: pagePath(who), textContent: fullName(who) });
  const playlistsOf = (who) => (me && who.id === me.id ? '/audio?section=playlists' : '/audio/' + encodeURIComponent(who.nickname) + '?section=playlists');
  if (playlistID) {
    const who = owner ?? (me && playlist?.ownerId === me.id ? me : null);
    return who
      ? [ownerLink(who), ' » ', el('a', { href: playlistsOf(who), textContent: tr('Плейлисты') }), ' » ', tr('Плейлист')]
      : [tr('Плейлист')];
  }
  if (section === 'new') return [tr('Аудиозаписи'), ' » ', tr('Новое')];
  if (section === 'playlists') return [tr('Аудиозаписи'), ' » ', mine() ? tr('Мои плейлисты') : tr('Плейлисты')];
  if (mine()) return [tr('Мои аудиозаписи')];

  return [ownerLink(owner), ' » ', tr('Аудиозаписи')];
}

// mark makes the link of the section shown dark, openvk's #used.
function mark(id, on) {
  $(id).classList.toggle('used', on);
}

// showTabs lays out the grey links at the right; a guest has only those
// of the page they look at.
function showTabs() {
  const other = Boolean(owner && !mine());
  $('title').replaceChildren(...heading());
  $('tabs').hidden = Boolean(playlistID);
  $('my-links').hidden = !me;
  $('owner-links').hidden = !other;
  $('owner-line').hidden = !me;
  if (other) {
    const base = '/audio/' + encodeURIComponent(owner.nickname);
    $('tab-owner').href = base;
    $('tab-owner-playlists').href = base + '?section=playlists';
  }
  mark('tab-mine', !other && section === '');
  mark('tab-new', section === 'new');
  mark('tab-playlists', !other && section === 'playlists');
  mark('tab-owner', other && section === '');
  mark('tab-owner-playlists', other && section === 'playlists');
  $('search-form').hidden = Boolean(playlistID);
}

function showPlaylistHead() {
  $('playlist-head').hidden = false;
  $('playlist-name').textContent = playlist.title;
  $('playlist-tools').hidden = !ownPlaylist();
  $('title').replaceChildren(...heading());
  // The songs go beside the cover, under the name, as in openvk.
  $('playlist-info').append($('audios'), $('empty'), $('more'));
}

function openDeletePlaylist() {
  const modal = openModal(tr('Удалить плейлист'));
  modal.body.append(el('p', { textContent: tr('Плейлист «{0}» будет удалён, а песни останутся в аудиозаписях. Удалить?', playlist.title) }));
  modal.actions.append(
    button(tr('Удалить'), async () => {
      modal.close();
      try {
        await api.deletePlaylist(playlist.id);
        location.href = '/audio?section=playlists';
      } catch (e) {
        showError($('error'), e);
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

function wire() {
  $('upload').addEventListener('click', () => uploadAudio($('error'), added));
  $('add-link').addEventListener('click', () => addByLink(added));
  $('new-playlist').addEventListener('click', () => newPlaylist((p) => {
    location.href = '/playlist/' + encodeURIComponent(p.id);
  }));
  $('playlist-upload').addEventListener('click', () => uploadAudio($('error'), addedToPlaylist));
  $('rename-playlist').addEventListener('click', () => nameWindow(tr('Редактировать плейлист'), playlist.title, tr('Сохранить'), async (title) => {
    playlist = { ...playlist, ...(await api.renamePlaylist(playlist.id, title)) };
    showPlaylistHead();
  }));
  $('delete-playlist').addEventListener('click', openDeletePlaylist);
  $('more').addEventListener('click', () => read(false));

  let typing = 0;
  $('search').addEventListener('input', () => {
    clearTimeout(typing);
    typing = setTimeout(() => {
      query = $('search').value.trim();
      $('playlists').hidden = !showsPlaylists();
      $('audios').hidden = showsPlaylists();
      if (showsPlaylists()) $('playlists').replaceChildren();
      read(true);
    }, 300);
  });
  $('search-form').addEventListener('submit', (event) => event.preventDefault());
}

me = await currentUser().catch(() => null);
renderChrome(me);

try {
  if (nickname) owner = await ownerRead;
  else if (!playlistID && me) owner = me;

  if (!owner && !playlistID && section !== 'new') {
    location.replace('/');
  } else {
    showTabs();
    wire();
    $('audios').hidden = showsPlaylists();
    $('playlists').hidden = !showsPlaylists();
    if (playlistID) {
      const page = await first;
      playlist = page;
      showPlaylistHead();
      showSongs(page, true);
      showEmpty();
    } else {
      await read(true, first);
    }
  }
} catch (e) {
  showError($('error'), e);
}
