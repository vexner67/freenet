// What the pages of photo albums share: their paths, the title "Анна »
// Альбомы » …" of openvk and who may change an album.

import { albumsCount, formatPostDate } from './labels.js';
import { adult, clubRef, el, fullName, pagePath } from './layout.js';

// albumsPath is the list of albums of owner, a person or a community (an
// author).
export function albumsPath(owner) {
  if (owner.club) return '/club/' + encodeURIComponent(clubRef(owner)) + '/albums';

  return '/albums?u=' + encodeURIComponent(owner.nickname);
}

export function albumPath(album) {
  return '/album/' + encodeURIComponent(album.id);
}

export function photoPath(photo) {
  return '/photo/' + encodeURIComponent(photo.id);
}

// albumsTitle is "<owner> » Альбомы » <what>…", the owner and the albums
// as links; what are strings or nodes.
export function albumsTitle(owner, ...what) {
  return [
    el('a', { href: pagePath(owner), textContent: fullName(owner) }),
    ' » ',
    el('a', { href: albumsPath(owner), textContent: 'Альбомы' }),
    ...what.flatMap((w) => [' » ', w]),
  ];
}

// canChange: the viewer me renames the albums of owner, deletes them and
// adds photos — their own, or a community's they run (role).
export function canChange(owner, role, me) {
  if (!me) return false;
  if (owner.club) return role === 'admin' || role === 'owner';

  return owner.id === me.id;
}

// isKeptBySite: "Фотографии со страницы" and "Фотографии со стены", which
// the site fills with every new photo of the page and of the posts on its
// wall; nobody renames, deletes or fills them by hand, as on openvk.
export function isKeptBySite(album) {
  return album.kind !== '';
}

// isAvatarAlbum: "Фотографии со страницы", where the photo of a page leads.
export function isAvatarAlbum(album) {
  return album.kind === 'avatars';
}

// coverURL is the photo that shows an album — the one chosen, else its
// first — or the camera of openvk.
export function coverURL(album) {
  return album.cover?.small ?? '/static/club.svg';
}

// coverNode is that photo, blurred when it came from an adult post.
export function coverNode(album, alt = '') {
  return adult(el('img', { src: coverURL(album), alt, loading: 'lazy' }), album.cover?.nsfw);
}

// albumsBlock is openvk's block "Альбомы" of a page: how many, "Все" and
// the newest with their covers and when they changed. Empty, it says
// "Загрузить" to whoever may add albums (canAdd), as on a community's page.
export function albumsBlock(owner, page, canAdd) {
  return [
    el('div', { className: 'section-title', textContent: 'Альбомы' }),
    el('div', { className: 'subtitle' },
      el('span', { textContent: albumsCount(page.total) }),
      el('a', { href: albumsPath(owner), textContent: 'Все' })),
    page.total === 0 && canAdd
      ? el('p', { className: 'side-text center' }, el('a', { href: albumsPath(owner), textContent: 'Загрузить' }))
      : el('div', { className: 'side-albums' }, ...page.albums.map((album) => el('div', { className: 'side-album' },
        el('a', { href: albumPath(album) }, coverNode(album)),
        el('div', {},
          el('a', { href: albumPath(album) }, el('b', { textContent: album.name })),
          el('div', { className: 'hint', textContent: 'Обновлено ' + formatPostDate(album.updatedAt) }),
        ),
      ))),
  ];
}
