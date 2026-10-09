// openvk's "Фон страницы" of a club at /club/<ref>/backdrop, for its
// admins (the form is backdrop.js's).

import { api, currentUser } from './api.js';
import { initBackdropForm } from './backdrop.js';
import { $, clubRef, el, renderChrome, showError } from './layout.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/backdrop'.length));

function render(club) {
  const path = '/club/' + encodeURIComponent(clubRef(club));

  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ' » Фон страницы');
  $('tab-main').href = path + '/edit';
  $('tab-backdrop').href = path + '/backdrop';
  $('tab-members').href = path + '/members';
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  location.replace('/');
} else {
  try {
    const club = await api.club(ref);
    document.title = `${club.name} | Фон страницы | Freenet`;
    render(club);

    if (club.role !== 'admin' && club.role !== 'owner') {
      showError($('error'), 'Менять фон группы могут только её администраторы.');
    } else {
      initBackdropForm(club.backdrop, {
        upload: async (side, file) => (await api.setClubBackdrop(ref, side, file)).backdrop,
        remove: async () => (await api.deleteClubBackdrop(ref)).backdrop,
      });
      $('tabs').hidden = false;
      $('form').hidden = false;
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
    showError($('error'), e);
  }
}
