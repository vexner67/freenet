// The page of an app (/app/<id>), as openvk's Apps/Play: the HTML widget
// over the width of the page, running at once, and under it its developer,
// how many added it, "Добавить к себе" or "Убрать из своих" and
// "Поделиться". A guest who follows a shared link plays too.

import { api, currentUser, started } from './api.js';
import { appUsersCount } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { htmlNode } from './htmlapp.js';
import { tr } from './i18n.js';

const id = decodeURIComponent(location.pathname.split('/')[2] ?? '');
const loading = started(api.app(id));

function showInstall(app) {
  const link = $('install');
  link.textContent = app.installed ? tr('Убрать из своих') : tr('Добавить к себе');
  link.hidden = false;
}

async function toggleInstall(app) {
  hideError($('error'));
  $('install').disabled = true;

  try {
    if (app.installed) await api.uninstallApp(app.id);
    else await api.installApp(app.id);
    app.users += app.installed ? -1 : 1;
    app.installed = !app.installed;
    showInstall(app);
    $('users').textContent = appUsersCount(app.users);
  } catch (e) {
    showError($('error'), e);
  } finally {
    $('install').disabled = false;
  }
}

// openShareApp puts the link of the app on your wall with your words.
function openShareApp(app, me) {
  const modal = openModal(tr('Поделиться'), { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const text = el('textarea', { maxLength: 59000, placeholder: '…', ariaLabel: tr('Ваш комментарий') });
  const send = button(tr('Отправить'), null, { type: 'submit' });

  modal.body.append(error, el('p', { textContent: tr('Ссылка на приложение «{0}» появится на Вашей стене.', app.name) }),
    el('b', { textContent: tr('Ваш комментарий') }), text);
  modal.actions.append(send, button(tr('Отмена'), modal.close, { secondary: true }));
  text.focus();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    send.disabled = true;

    const link = `${location.origin}/app/${encodeURIComponent(app.id)}`;
    try {
      await api.createPost(me.nickname, [text.value.trim(), link].filter(Boolean).join('\n'));
      modal.close();
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const app = await loading;
  document.title = `${app.name} | Freenet`;
  $('title').textContent = app.name;
  $('app').append(htmlNode(app.document, { page: true }));

  if (app.developer) $('developer').append(el('a', { href: pagePath(app.developer), textContent: fullName(app.developer) }));
  else $('developer').textContent = tr('страница удалена');
  $('users').textContent = appUsersCount(app.users);

  if (me?.nickname) {
    showInstall(app);
    $('install').addEventListener('click', () => toggleInstall(app));
    $('share').hidden = false;
    $('share').addEventListener('click', () => openShareApp(app, me));
  }
  $('about').hidden = false;
} catch (e) {
  showError($('error'), e);
}
