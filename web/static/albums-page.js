// "Альбомы" of a person or a community, as openvk's: "Мои Фотографии" for
// your own, "Анна » Альбомы" otherwise; how many there are with "Создать
// альбом" for whoever may, and the albums, newest first, each with its
// cover, name, description, how many photos and when it was changed and
// created. /albums is yours, /albums?u=<nickname> someone's and
// /club/<ref>/albums a community's.

import { api, currentUser } from './api.js';
import { albumsCount, formatPostDate, myAlbumsCount, photosCount } from './labels.js';
import { $, clubRef, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { albumPath, canChange, coverNode } from './albums.js';

const clubPage = location.pathname.startsWith('/club/');
const ref = clubPage ? decodeURIComponent(location.pathname.slice('/club/'.length, -'/albums'.length)) : '';
const nickname = new URLSearchParams(location.search).get('u') ?? '';

let cursor = '';

function albumNode(album) {
  const path = albumPath(album);

  return el('div', { className: 'album-item' },
    el('a', { className: 'album-cover', href: path }, coverNode(album, album.name)),
    el('div', { className: 'album-info' },
      el('a', { href: path }, el('b', { textContent: album.name })),
      el('div', { className: 'list-description', textContent: album.description || album.name }),
      el('div', { className: 'hint', textContent: photosCount(album.photos) }),
      el('div', { className: 'hint', textContent: 'Обновлено ' + formatPostDate(album.updatedAt) }),
      el('div', { className: 'hint', textContent: 'Создано ' + formatPostDate(album.createdAt) }),
    ),
  );
}

// renderHead names the albums and says how many there are, with "Создать
// альбом" for whoever may create one.
function renderHead(page, me) {
  const { owner, role } = page;
  const mine = !owner.club && owner.id === me?.id;

  document.title = `Альбомы ${fullName(owner)} | Freenet`;
  if (mine) $('title').textContent = 'Мои Фотографии';
  else $('title').replaceChildren(el('a', { href: pagePath(owner), textContent: fullName(owner) }), ' » Альбомы');

  $('summary-count').textContent = mine ? myAlbumsCount(page.total) : albumsCount(page.total);
  if (canChange(owner, role, me)) {
    const create = '/albums/create' + (owner.club ? '?club=' + encodeURIComponent(clubRef(owner)) : '');
    $('summary-links').replaceChildren(' | ', el('a', { href: create, textContent: 'Создать альбом' }));
  }
  $('summary').hidden = false;
}

async function loadMore(source, me) {
  hideError($('error'));

  try {
    const page = await api.albums({ ...source, cursor });

    if (!cursor) {
      renderHead(page, me);
      if (page.albums.length === 0) $('albums').append(el('p', { className: 'hint', textContent: 'Тут ничего нет... Пока...' }));
      $('albums').hidden = false;
    }

    $('albums').append(...page.albums.map(albumNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    if (!cursor) $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Альбомы';
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!ref && !nickname && !me) {
  location.replace('/');
} else if (!ref && !nickname && !me.profileCompleted) {
  $('title').textContent = 'Мои Фотографии';
  showError($('error'), 'Сначала заполните свою страницу.');
} else {
  const source = ref ? { club: ref } : { nickname: nickname || me.nickname };

  $('more').addEventListener('click', () => loadMore(source, me));
  await loadMore(source, me);
}
