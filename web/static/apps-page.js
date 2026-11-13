// "Приложения" (/apps), as openvk's: the tabs "Все приложения" (the HTML
// widgets someone added), "Мои приложения" (?act=installed, those you
// added) and "Управление" (?act=own, the widgets you uploaded), each app
// a row of the lists: its name, developer and users, with "Запустить" and
// "Добавить к себе" or "Убрать из своих".

import { api, currentUser, started } from './api.js';
import { appUsersCount } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const act = new URLSearchParams(location.search).get('act');
const section = act === 'installed' || act === 'own' ? act : 'all';
const first = started(api.apps(section));
let cursor = '';

// installLink adds the app to yours or takes it away, in place.
function installLink(app) {
  const link = el('button', { className: 'profile-link', type: 'button' });
  const render = () => { link.textContent = app.installed ? tr('Убрать из своих') : tr('Добавить к себе'); };

  link.addEventListener('click', async () => {
    hideError($('error'));
    link.disabled = true;

    try {
      if (app.installed) await api.uninstallApp(app.id);
      else await api.installApp(app.id);
      app.installed = !app.installed;
      render();
    } catch (e) {
      showError($('error'), e);
    } finally {
      link.disabled = false;
    }
  });
  render();

  return link;
}

function appRow(app) {
  const path = `/app/${encodeURIComponent(app.id)}`;

  return el('div', { className: 'list-row' },
    el('a', { className: 'list-cover app-cover', href: path }, el('img', { src: '/static/icons/widget.svg', alt: '' })),
    el('div', { className: 'list-info' },
      el('div', { className: 'club-list-name' }, el('a', { href: path, textContent: app.name })),
      el('table', { className: 'info app-info' },
        el('tr', {}, el('th', { textContent: tr('Название:') }), el('td', {}, el('a', { href: path, textContent: app.name }))),
        app.developer && el('tr', {}, el('th', { textContent: tr('Разработчик:') }),
          el('td', {}, el('a', { href: pagePath(app.developer), textContent: fullName(app.developer) }))),
        el('tr', {}, el('th', { textContent: tr('Пользователей:') }), el('td', { textContent: appUsersCount(app.users) })),
      ),
    ),
    el('div', { className: 'list-actions' },
      el('a', { className: 'profile-link', href: path, textContent: tr('Запустить') }),
      installLink(app)),
  );
}

async function loadMore(page) {
  $('more').hidden = true;
  hideError($('error'));

  try {
    const { apps, nextCursor } = await (page ?? api.apps(section, cursor));
    $('apps').append(...apps.map(appRow));
    cursor = nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('apps').childElementCount > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  $(`tab-${section}`).classList.add('active');
  $('about').hidden = section !== 'own';
  $('more').addEventListener('click', () => loadMore());
  loadMore(first);
}
