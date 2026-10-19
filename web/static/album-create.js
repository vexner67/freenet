// openvk's "Создание альбома" at /albums/create, ?club=<ref> for an album
// of a community: the name and the description; the new album opens then.

import { api, ApiError, currentUser } from './api.js';
import { $, renderChrome, showError, hideError } from './layout.js';
import { albumPath, albumsTitle, canChange } from './albums.js';
import { tr } from './i18n.js';

const ref = new URLSearchParams(location.search).get('club') ?? '';

// ownerOf is whom the album will belong to, with the viewer's role in it.
async function ownerOf(me) {
  if (!ref) return { owner: me, role: '' };

  const club = await api.club(ref);

  return { owner: { ...club, club: true }, role: club.role };
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  location.replace('/');
} else if (!ref && !me.profileCompleted) {
  $('title').textContent = tr('Создание альбома');
  showError($('error'), tr('Сначала заполните свою страницу.'));
} else {
  try {
    const { owner, role } = await ownerOf(me);

    $('title').replaceChildren(...albumsTitle(owner, tr('Создание альбома')));
    if (!canChange(owner, role, me)) throw new Error(tr('Создавать альбомы группы могут только её администраторы.'));
    $('content').hidden = false;

    $('form').addEventListener('submit', async (event) => {
      event.preventDefault();
      hideError($('error'));
      $('submit').disabled = true;

      try {
        const album = await api.createAlbum($('name').value, $('description').value, { club: ref });
        location.assign(albumPath(album));
      } catch (e) {
        showError($('error'), e);
        $('submit').disabled = false;
      }
    });
  } catch (e) {
    if (e.status === 404) $('title').textContent = tr('Группа не найдена');
    showError($('error'), e instanceof ApiError ? e : e.message);
  }
}
