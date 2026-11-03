// "Аудиозаписи" (/audio, /audio/<nickname>, /playlist/<id>), as openvk's
// audios page: "Моя музыка" — the songs one keeps, the last added first;
// "Новое" — every song of the site, the newest first; "Плейлисты"; a
// search of all songs over them; "Загрузить аудио" and "Добавить по
// ссылке" for one's own. Someone else's page shows their songs and
// playlists, a playlist its songs.

import { api, currentUser, started } from './api.js';
import { audiosCount, audioSite, genitiveName, formatPostDate } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { openModal, button, toast } from './modal.js';
import { addButton, addByLink, audioNode, confirmDelete, editAudio, keep, nameWindow, newPlaylist, pickPlaylist, uploadAudio } from './audio.js';
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

const mine = () => Boolean(me && owner && owner.id === me.id);
const ownPlaylist = () => Boolean(me && playlist && playlist.ownerId === me.id);

function showSummary() {
  const summary = $('summary');
  summary.hidden = query !== '' || section === 'new' || section === 'playlists' || total === 0;
  summary.textContent = mine() && !playlist ? tr('У Вас ') + audiosCount(total) : audiosCount(total);
  $('empty').hidden = $('audios').childElementCount !== 0 || $('playlists').childElementCount !== 0;
}

// items is "⋯" of a song: what this reader may do with it here.
function items(audio, row) {
  const list = [];
  if (me) {
    if (mine() && !playlist && !query && section !== 'new') {
      list.push({ text: tr('Удалить из моих аудиозаписей'), run: () => forget(audio, row) });
    } else if (!audio.added) {
      list.push({ text: tr('Добавить к себе'), run: () => keep(audio) });
    }
    list.push({ text: tr('Добавить в плейлист'), run: () => pickPlaylist(audio) });
    if (ownPlaylist()) list.push({ text: tr('Убрать из плейлиста'), run: () => removeFromPlaylist(audio, row) });
    list.push({ text: tr('Сделать песней страницы'), run: () => setPageAudio(audio) });
    if (audio.kind === 'file' && audio.ownerId === me.id) {
      list.push({ text: tr('Редактировать'), run: () => editAudio(audio, (edited) => replace(audio, row, edited)) });
      list.push({ text: tr('Удалить'), danger: true, run: () => confirmDelete(audio, () => gone(audio, row)) });
    }
  }
  if (audio.link) list.push({ text: tr('Слушать на {0}', audioSite(audio)), run: () => window.open(audio.link, '_blank', 'noopener') });

  return list;
}

function rowOf(audio) {
  const quick = me && !audio.added && !(mine() && !playlist) ? [addButton(audio)] : [];
  const row = audioNode(audio, { queue: () => shown, tools: quick, menu: () => items(audio, row) });

  return row;
}

function replace(audio, row, edited) {
  shown[shown.indexOf(audio)] = edited;
  row.replaceWith(rowOf(edited));
}

function gone(audio, row) {
  shown = shown.filter((a) => a !== audio);
  row.remove();
  total--;
  showSummary();
}

async function forget(audio, row) {
  try {
    await api.forgetAudio(audio.id);
    gone(audio, row);
  } catch (e) {
    showError($('error'), e);
  }
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
  if (!mine() || playlist || query || section) return;

  shown.unshift(audio);
  $('audios').prepend(rowOf(audio));
  total++;
  showSummary();
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

function showPlaylists(page) {
  $('playlists').hidden = false;
  $('playlists').append(...page.playlists.map((p) => el('a', { className: 'playlist-item', href: '/playlist/' + encodeURIComponent(p.id) },
    el('span', { className: 'playlist-cover' }),
    el('span', { className: 'playlist-name', textContent: p.title }),
    el('span', { className: 'playlist-date', textContent: formatPostDate(p.createdAt) }))));
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

    if (section === 'playlists' && !query) showPlaylists(page);
    else showSongs(page, fresh);
    showSummary();
  } catch (e) {
    showError($('error'), e);
  }
}

// showTabs marks the section; someone else's page names whose it is.
function showTabs() {
  const tab = playlistID ? '' : section === 'new' ? 'tab-new' : section === 'playlists' ? 'tab-playlists' : 'tab-mine';
  for (const a of $('tabs').querySelectorAll('a[id^="tab-"]')) a.classList.toggle('active', a.id === tab);

  const base = owner && !mine() ? '/audio/' + encodeURIComponent(owner.nickname) : '/audio';
  $('tab-mine').href = base;
  $('tab-playlists').href = base + '?section=playlists';
  if (owner && !mine()) {
    $('tab-mine').textContent = tr('Аудиозаписи');
    $('title').textContent = tr('Аудиозаписи {0}', genitiveName(owner));
  }
  $('tab-new').hidden = Boolean(owner && !mine());
  // A guest has no music of their own to go to.
  $('tab-mine').hidden = $('tab-playlists').hidden = !owner && !me;
  for (const id of ['upload', 'add-link']) $(id).hidden = !me;
  $('new-playlist').hidden = !(me && (mine() || !owner) && section === 'playlists');
}

function showPlaylistHead() {
  $('title').textContent = playlist.title;
  $('playlist-tools').hidden = !ownPlaylist();
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
  $('rename-playlist').addEventListener('click', () => nameWindow(tr('Переименовать плейлист'), playlist.title, tr('Сохранить'), async (title) => {
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
      $('playlists').hidden = Boolean(query) || section !== 'playlists';
      $('audios').hidden = !query && section === 'playlists';
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
    if (playlistID) {
      const page = await first;
      playlist = page;
      showPlaylistHead();
      showSongs(page, true);
      showSummary();
    }
    showTabs();
    wire();
    $('audios').hidden = section === 'playlists';
    if (!playlistID) await read(true, first);
  }
} catch (e) {
  showError($('error'), e);
}
