// Songs, as openvk's "Аудиозаписи": a song is a row with a grey play
// button, "Исполнитель — Название" and its length; "⋯" at its right end
// keeps what may be done with it. Posts and messages attach songs
// ("Прикрепить" → "Аудиозапись"), picked from one's library, uploaded or
// added by a link of SoundCloud or YouTube there and then.
//
// The songs play in the player (player.js), made at the first song of
// the tab; while it plays, the pages opened from this one open in the
// frame of shell.js, and their songs play in the player of the tab.

import { api, currentUser, signedIn } from './api.js';
import { MAX_AUDIOS, MAX_AUDIO_BYTES, audioArtist, audioNames, audioSite, audioTime, audioTitle, errorText } from './labels.js';
import { el, externalLink, isPhone, showError, hideError } from './layout.js';
import { button, openMenu, openModal, openSheet, toast } from './modal.js';
import { tr } from './i18n.js';

// What plays now and the songs that would not, as the player last said.
const playing = { id: '', on: false };
const broken = new Set();

// outer is the player of the tab when this page is in the frame of
// shell.js: songs and what plays go to it and from it as JSON, so that
// nothing of a page that goes away stays in it.
const outer = (() => {
  try {
    return window.parent !== window ? window.parent.freenetPlayer ?? null : null;
  } catch {
    return null;
  }
})();

// player is the player of this tab, made at its first song.
let player = null;

async function bar() {
  if (outer) return outer;
  if (player) return player;

  const [{ createPlayer }, shell] = await Promise.all([import('/static/player.js'), import('/static/shell.js')]);
  let state = playing;
  const made = createPlayer(document.body, {
    onState: (next) => {
      state = next;
      showState(next);
      shell.tellFrame({ freenetPlayer: next });
    },
  });
  shell.startShell(made.active);
  player = {
    play: (audio, queue) => made.play(JSON.parse(audio), JSON.parse(queue)),
    toggle: () => made.toggle(),
    state: () => JSON.stringify(state),
    restore: (saved) => made.restore(saved),
  };
  window.freenetPlayer = player;
  return player;
}

if (outer) {
  // The page in the frame hears what plays from the tab.
  window.addEventListener('message', (event) => {
    if (event.origin === location.origin && event.source === window.parent && event.data?.freenetPlayer) showState(event.data.freenetPlayer);
  });
  showState(JSON.parse(outer.state()));
} else {
  // A song the last page of the tab played plays on here.
  import('/static/player.js').then(({ saved }) => {
    const state = saved();
    if (state) bar().then((p) => p.restore(state));
  }).catch(() => {});
}

// showState marks the rows of the song that plays and of those that
// would not.
export function showState({ id = '', on = false, broken: gone = [] }) {
  playing.id = id;
  playing.on = on;
  for (const b of gone) broken.add(b);

  for (const row of document.querySelectorAll('.audio-row')) markRow(row);
}

function markRow(row) {
  const id = row.dataset.audio;
  row.classList.toggle('playing', id === playing.id);
  row.classList.toggle('paused', id === playing.id && !playing.on);
  row.classList.toggle('broken', broken.has(id));
}

// play plays audio and after it the rest of queue, the list it is in;
// the song that plays pauses or goes on.
export async function play(audio, queue = [audio]) {
  const p = await bar();
  if (audio.id === playing.id) p.toggle();
  else p.play(JSON.stringify(audio), JSON.stringify(queue));
}

// audioNode is a song as a row. queue gives the list it plays in; pick,
// when given, makes a click on the name choose it (the picker of a form);
// tools are links at the right end, as the cross of an attached song;
// menu adds to "⋯" what the page may do with it (items of openMenu).
export function audioNode(audio, { queue = () => [audio], pick = null, tools = [], menu = null } = {}) {
  const name = el('span', { className: 'audio-name' },
    el('b', { className: 'audio-artist', textContent: audioArtist(audio) }),
    ' — ',
    el('span', { className: 'audio-title', textContent: audioTitle(audio) }));
  const length = audio.duration
    ? el('span', { className: 'audio-time', textContent: audioTime(audio.duration) })
    : el('span', { className: 'audio-time audio-site', textContent: audio.kind === 'file' ? '' : audioSite(audio) });

  const row = el('div', { className: 'audio-row' },
    el('button', { type: 'button', className: 'audio-play', title: tr('Слушать'), ariaLabel: tr('Слушать'), onclick: () => play(audio, queue()) }),
    pick
      ? el('a', { className: 'audio-pick', onclick: () => pick(audio) }, name)
      : el('span', { className: 'audio-label', onclick: () => play(audio, queue()) }, name),
    el('span', { className: 'audio-broken', textContent: tr('Трек недоступен') }),
    length,
    ...tools,
    menu && el('button', { type: 'button', className: 'audio-more', title: tr('Действия'), ariaLabel: tr('Действия'), textContent: '⋯', onclick: (event) => openActions(event.currentTarget, menu()) }),
  );
  row.dataset.audio = audio.id;
  markRow(row);

  return row;
}

function openActions(anchor, items) {
  if (isPhone()) openSheet(items);
  else openMenu(anchor, items);
}

// audiosNode is the songs a post or a message attaches, under its words
// and photos; null for none. Each plays the list on from itself.
export function audiosNode(audios) {
  if (!audios?.length) return null;

  const box = el('div', { className: 'audio-list' });
  box.append(...audios.map((a) => audioNode(a, {
    queue: () => audios,
    tools: signedIn() && !a.added ? [addButton(a)] : [],
    menu: () => libraryItems(a),
  })));

  return box;
}

// libraryItems is "⋯" of a song somewhere else than one's library:
// "Добавить к себе" for the signed in, and its site for a track.
function libraryItems(audio) {
  const items = [];
  if (signedIn()) {
    items.push({
      text: audio.added ? tr('Уже в ваших аудиозаписях') : tr('Добавить к себе'),
      run: () => keep(audio),
    });
  }
  if (audio.link) items.push({ text: tr('Слушать на {0}', audioSite(audio)), run: () => window.open(audio.link, '_blank', 'noopener') });
  items.push({ text: tr('Добавить в плейлист'), run: () => pickPlaylist(audio) });

  return items;
}

// addButton is "+" of a song one does not keep: a click adds it, and it
// becomes a tick.
export function addButton(audio) {
  const add = el('button', {
    type: 'button', className: 'audio-add', title: tr('Добавить к себе'), ariaLabel: tr('Добавить к себе'), textContent: '+',
    onclick: async () => {
      add.disabled = true;
      await keep(audio);
      if (!audio.added) {
        add.disabled = false;
        return;
      }
      add.textContent = '✓';
      add.title = add.ariaLabel = tr('В ваших аудиозаписях');
      add.classList.add('done');
    },
  });

  return add;
}

// keep is "Добавить к себе".
export async function keep(audio) {
  try {
    await api.keepAudio(audio.id);
    audio.added = true;
    toast(tr('Аудиозапись добавлена к вам.'));
  } catch (e) {
    toast(errorText(e));
  }
}

// pickPlaylist is "Добавить в плейлист": one's playlists, a click puts
// the song there; "Новый плейлист" makes one with it.
export async function pickPlaylist(audio) {
  const me = await currentUser().catch(() => null);
  if (!me) {
    toast(tr('Войдите на сайт, чтобы собирать плейлисты.'));
    return;
  }

  const modal = openModal(tr('Добавить в плейлист'));
  const error = el('div', { className: 'error', hidden: true });
  const list = el('div', { className: 'playlist-picker' });
  modal.body.append(error, list);

  const add = async (playlist) => {
    try {
      await api.addToPlaylist(playlist.id, audio.id);
      modal.close();
      toast(tr('Аудиозапись добавлена в плейлист «{0}».', playlist.title));
    } catch (e) {
      showError(error, e);
    }
  };

  modal.actions.append(
    button(tr('Новый плейлист'), () => {
      modal.close();
      newPlaylist(async (p) => {
        await api.addToPlaylist(p.id, audio.id);
        toast(tr('Аудиозапись добавлена в плейлист «{0}».', p.title));
      });
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );

  try {
    const page = await api.playlists(me.nickname);
    list.append(...page.playlists.map((p) => el('a', { className: 'playlist-pick', textContent: p.title, onclick: () => add(p) })));
    if (!page.playlists.length) list.append(el('p', { className: 'hint', textContent: tr('У вас пока нет плейлистов.') }));
  } catch (e) {
    showError(error, e);
  }
}

// newPlaylist is "Новый плейлист": its title, then onDone with it.
export function newPlaylist(onDone) {
  nameWindow(tr('Новый плейлист'), '', tr('Создать'), async (title) => {
    const p = await api.createPlaylist(title);
    await onDone(p);
  });
}

// nameWindow asks for one title: of a playlist made or renamed.
export function nameWindow(heading, value, action, save) {
  const modal = openModal(heading, { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const input = el('input', { type: 'text', maxLength: 128, value, ariaLabel: tr('Название') });
  const submit = button(action, null, { type: 'submit' });

  modal.body.append(error, el('p', {}, el('b', { textContent: tr('Название') })), input);
  modal.actions.append(submit, button(tr('Отмена'), modal.close, { secondary: true }));
  input.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    submit.disabled = true;

    try {
      await save(input.value);
      modal.close();
    } catch (e) {
      showError(error, e);
      submit.disabled = false;
    }
  });
}

// durationOf reads how long an MP3 plays, 0 when the browser cannot.
async function durationOf(file) {
  const url = URL.createObjectURL(file);
  const probe = new Audio();
  probe.preload = 'metadata';

  try {
    await new Promise((resolve, reject) => {
      probe.addEventListener('loadedmetadata', resolve, { once: true });
      probe.addEventListener('error', reject, { once: true });
      probe.src = url;
    });
    return Number.isFinite(probe.duration) ? Math.round(probe.duration) : 0;
  } catch {
    return 0;
  } finally {
    URL.revokeObjectURL(url);
  }
}

// uploadAudio is openvk's "Загрузить аудио": an MP3, then its artist and
// title, read from the file name ("Исполнитель - Название.mp3") and
// changed at will; onDone gets the song.
export function uploadAudio(error, onDone) {
  const input = el('input', { type: 'file', accept: '.mp3,audio/mpeg', hidden: true });

  input.addEventListener('change', () => {
    const file = input.files[0];
    input.remove();
    if (!file) return;

    hideError(error);
    if (file.size > MAX_AUDIO_BYTES) {
      showError(error, tr('Файл слишком большой: не больше 20 МБ.'));
      return;
    }
    if (file.size === 0) {
      showError(error, tr('Файл пустой.'));
      return;
    }

    openUpload(file, onDone);
  });

  document.body.append(input);
  input.click();
}

function openUpload(file, onDone) {
  const names = audioNames(file.name);
  const modal = openModal(tr('Загрузка аудиозаписи'), { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const artist = el('input', { type: 'text', maxLength: 128, value: names.artist, ariaLabel: tr('Исполнитель') });
  const title = el('input', { type: 'text', maxLength: 128, value: names.title, ariaLabel: tr('Название') });
  const submit = button(tr('Загрузить'), null, { type: 'submit' });

  modal.body.append(error,
    el('p', {}, el('b', { textContent: tr('Исполнитель') })), artist,
    el('p', {}, el('b', { textContent: tr('Название') })), title);
  modal.actions.append(submit, button(tr('Отмена'), modal.close, { secondary: true }));
  title.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    submit.disabled = true;
    submit.textContent = tr('Загрузка…');

    try {
      const audio = await api.uploadAudio(file, { artist: artist.value, title: title.value, duration: await durationOf(file) });
      modal.close();
      onDone(audio);
    } catch (e) {
      showError(error, e);
      submit.disabled = false;
      submit.textContent = tr('Загрузить');
    }
  });
}

// addByLink is "Добавить по ссылке": a track of SoundCloud or YouTube
// goes to one's library. One its site does not let other sites play is
// refused, with the way to hear it there.
export function addByLink(onDone) {
  const modal = openModal(tr('Добавить по ссылке'), { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const refused = el('div', { className: 'audio-refused', hidden: true });
  const input = el('input', { type: 'url', placeholder: 'https://soundcloud.com/…', ariaLabel: tr('Ссылка на трек') });
  const submit = button(tr('Добавить'), null, { type: 'submit' });

  modal.body.append(error, refused,
    el('p', { className: 'hint', textContent: tr('Ссылка на трек SoundCloud или видео YouTube.') }), input);
  modal.actions.append(submit, button(tr('Отмена'), modal.close, { secondary: true }));
  input.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    refused.hidden = true;
    submit.disabled = true;

    try {
      const audio = await api.addTrack(input.value.trim());
      modal.close();
      onDone(audio);
    } catch (e) {
      if (e.message === 'this track cannot be played on other sites') {
        const site = /youtu/i.test(input.value) ? 'YouTube' : 'SoundCloud';
        refused.replaceChildren(
          el('p', { textContent: tr('Этот трек нельзя слушать на Freenet: его автор запретил играть его на других сайтах.') }),
          externalLink(input.value.trim(), [tr('Слушать на {0}', site)]));
        refused.hidden = false;
      } else {
        showError(error, e);
      }
    } finally {
      submit.disabled = false;
    }
  });
}

// editAudio is openvk's "Редактирование аудиозаписи" of one's upload;
// onDone gets the song as it is now.
export function editAudio(audio, onDone) {
  const modal = openModal(tr('Редактирование аудиозаписи'), { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const artist = el('input', { type: 'text', maxLength: 128, value: audio.artist, ariaLabel: tr('Исполнитель') });
  const title = el('input', { type: 'text', maxLength: 128, value: audio.title, ariaLabel: tr('Название') });
  const submit = button(tr('Сохранить'), null, { type: 'submit' });

  modal.body.append(error,
    el('p', {}, el('b', { textContent: tr('Исполнитель') })), artist,
    el('p', {}, el('b', { textContent: tr('Название') })), title);
  modal.actions.append(submit, button(tr('Отмена'), modal.close, { secondary: true }));
  artist.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    submit.disabled = true;

    try {
      onDone(await api.editAudio(audio.id, artist.value, title.value));
      modal.close();
    } catch (e) {
      showError(error, e);
      submit.disabled = false;
    }
  });
}

// confirmDelete asks first: the song goes for everyone who keeps it and
// from the posts and messages that attach it.
export function confirmDelete(audio, onDone) {
  const modal = openModal(tr('Удалить аудиозапись'));
  modal.body.append(el('p', { textContent: tr('Аудиозапись «{0}» пропадёт у всех, кто её добавил, и из записей и сообщений. Удалить?', audioTitle(audio)) }));
  modal.actions.append(
    button(tr('Удалить'), async () => {
      modal.close();
      try {
        await api.deleteAudio(audio.id);
        onDone();
      } catch (e) {
        toast(errorText(e));
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

// openPicker is openvk's "Выбрать аудиозаписи" (showAudioAttachment in
// Web/static/js/al_music.js): a search over the grey strip at the top,
// one's library under it — or what is found — and at the right of each song
// "Прикрепить аудиозапись", which turns into "Открепить аудиозапись"; the
// window stays open, so several are attached at once. attached tells
// whether a song is attached, toggle attaches it or takes it off.
async function openPicker({ attached, toggle }) {
  const modal = openModal(tr('Выбрать аудиозаписи'));
  const error = el('div', { className: 'error', hidden: true });
  const search = el('input', { type: 'text', maxLength: 50, placeholder: tr('Поиск'), ariaLabel: tr('Поиск аудиозаписей') });
  const list = el('div', { className: 'audios-insert' });
  const more = el('a', { className: 'show-more', textContent: tr('Показать больше аудиозаписей'), hidden: true });
  const me = await currentUser().catch(() => null);
  let cursor = '';
  let query = '';
  let asked = 0;

  const label = (audio) => (attached(audio) ? tr('Открепить аудиозапись') : tr('Прикрепить аудиозапись'));

  function row(audio, page) {
    const text = el('span', { textContent: label(audio) });
    return el('div', { className: 'audio-attach-row' },
      el('div', { className: 'audio-attach-player' }, audioNode(audio, { queue: () => page.audios })),
      el('a', {
        className: 'audio-attach-toggle',
        onclick: () => {
          if (!toggle(audio)) return;
          text.textContent = label(audio);
        },
      }, text));
  }

  async function load(fresh) {
    const ask = ++asked;
    more.hidden = true;
    if (fresh) cursor = '';

    try {
      const page = query ? await api.searchAudios(query, cursor) : await api.audios(me.nickname, cursor);
      if (ask !== asked) return;
      if (fresh) list.replaceChildren();
      cursor = page.nextCursor;
      list.append(...page.audios.map((a) => row(a, page)));
      if (!list.childElementCount) list.append(el('p', { className: 'hint', textContent: query ? tr('Результатов нет') : tr('Вы ещё не добавляли аудиозаписей.') }));
      more.hidden = !cursor;
      list.append(more);
    } catch (e) {
      showError(error, e);
    }
  }

  let typing = 0;
  search.addEventListener('input', () => {
    clearTimeout(typing);
    typing = setTimeout(() => {
      query = search.value.trim();
      load(true);
    }, 300);
  });
  more.addEventListener('click', () => load(false));
  modal.box.classList.add('audio-attach');
  modal.body.append(el('div', { className: 'audio-attach-search' }, search), error, list);
  modal.actions.append(button(tr('Закрыть'), modal.close, { secondary: true }));
  load(true);
}

// initAudios wires "Аудиозапись" of the attach menu of a form: link
// opens the picker, list shows what is attached, each with a cross;
// onChange runs whenever songs come or go, not when the form is made.
export function initAudios({ link, list, onChange = () => {} }) {
  let audios = [];

  function render(changed = true) {
    list.replaceChildren(...audios.map((a) => audioNode(a, {
      queue: () => audios,
      tools: [el('a', { className: 'doc-remove', title: tr('Не прикреплять'), textContent: '×', onclick: () => remove(a) })],
    })));
    list.hidden = audios.length === 0;
    link.hidden = audios.length >= MAX_AUDIOS;
    if (changed) onChange();
  }

  function remove(audio) {
    audios = audios.filter((a) => a !== audio);
    render();
  }

  link.addEventListener('click', () => openPicker({
    attached: (audio) => audios.some((a) => a.id === audio.id),
    // A song more than the form takes is not attached, and says so.
    toggle(audio) {
      if (audios.some((a) => a.id === audio.id)) {
        audios = audios.filter((a) => a.id !== audio.id);
      } else if (audios.length < MAX_AUDIOS) {
        audios.push(audio);
      } else {
        toast(tr('Можно прикрепить не больше {0} аудиозаписей.', MAX_AUDIOS));
        return false;
      }
      render();
      return true;
    },
  }));

  render(false);

  return {
    ids: () => audios.map((a) => a.id),
    count: () => audios.length,
    clear() {
      audios = [];
      render();
    },
  };
}
