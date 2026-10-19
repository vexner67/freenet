// openvk's "Фон страницы" of a club at /club/<ref>/backdrop, for its
// admins (the form is backdrop.js's).

import { api, currentUser } from './api.js';
import { initBackdropForm } from './backdrop.js';
import { $, clubRef, el, renderChrome, showError } from './layout.js';
import { tr } from './i18n.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/backdrop'.length));

function render(club) {
  const path = '/club/' + encodeURIComponent(clubRef(club));

  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), tr(' » Фон страницы'));
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
    document.title = tr('{0} | Фон страницы | Freenet', club.name);
    render(club);

    if (club.role !== 'admin' && club.role !== 'owner') {
      showError($('error'), tr('Менять фон группы могут только её администраторы.'));
    } else {
      initBackdropForm(club.backdrop, {
        upload: async (side, file) => (await api.setClubBackdrop(ref, side, file)).backdrop,
        remove: async () => (await api.deleteClubBackdrop(ref)).backdrop,
      });
      $('tabs').hidden = false;
      $('form').hidden = false;
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? tr('Группа не найдена') : tr('Ошибка');
    showError($('error'), e);
  }
}
