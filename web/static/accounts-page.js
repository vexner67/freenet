// "Аккаунты" (/accounts): the accounts of this browser. openvk has no
// such page, so it is laid out as openvk lays out its lists of people
// ("Мои Друзья", @listView.latte): the summary bar with how many there
// are, then a row for each — the photo, a table of "Имя:" and the like,
// and the links on the right. The account signed in comes first, with
// "Выйти"; each that waits has "Войти", which switches to it without
// signing out. "Добавить аккаунт" sets the one signed in aside and opens
// the page of signing in, while there is room for one more.

import { currentUser } from './api.js';
import { MAX_PARKED, accountName, parkedAccounts } from './accounts.js';
import { plural } from './labels.js';
import { $, addAccount, avatarURL, el, fullName, logout, renderChrome, showAccounts, switchAccount } from './layout.js';
import { tr } from './i18n.js';

function accountsCount(n) {
  return `${n} ${plural(n, ['аккаунт', 'аккаунта', 'аккаунтов'])}`;
}

function line(label, value) {
  return el('tr', {}, el('th', { textContent: label }), el('td', {}, value));
}

// accountRow is a row of the list: what the browser knows of the account
// and its links.
function accountRow({ photo, name, nickname, unread = 0 }, ...actions) {
  const path = nickname ? '/' + encodeURIComponent(nickname) : '';

  return el('div', { className: 'list-row' },
    el(path ? 'a' : 'span', { className: 'list-cover', href: path || undefined }, el('img', { src: photo, alt: '' })),
    el('div', { className: 'list-info' },
      el('table', { className: 'info' },
        line(tr('Имя:'), path ? el('a', { href: path, textContent: name }) : name),
        nickname && line(tr('Адрес:'), '@' + nickname),
        unread > 0 && line(tr('Непрочитанных:'), String(unread)))),
    el('div', { className: 'list-actions' }, ...actions));
}

// signIn is "Войти" of an account that waits; it stays pressed while the
// page goes.
function signIn(account, me) {
  const link = el('button', { className: 'profile-link', type: 'button', textContent: tr('Войти') });
  link.addEventListener('click', () => {
    link.disabled = true;
    switchAccount(account.id, me).finally(() => {
      link.disabled = false;
    });
  });

  return link;
}

function draw(me) {
  const waiting = parkedAccounts();
  const total = waiting.length + 1;
  const free = MAX_PARKED - waiting.length;

  $('summary').textContent = tr('В этом браузере {0}', accountsCount(total));
  $('room').textContent = free > 0 ? tr('Можно добавить ещё {0}', accountsCount(free)) : tr('Больше аккаунтов добавить нельзя.');
  $('add').hidden = free <= 0;
  $('accounts').replaceChildren(
    accountRow({ photo: avatarURL(me, 'small'), name: fullName(me) || tr('Новая страница'), nickname: me.nickname },
      el('span', { className: 'list-here', textContent: tr('Вы вошли в этот аккаунт') }),
      el('a', { className: 'profile-link', href: '/', textContent: tr('Выйти'), onclick: logout })),
    ...waiting.map((a) => accountRow(
      { photo: a.avatar || '/static/avatar.svg', name: accountName(a), nickname: a.nickname, unread: a.unread },
      signIn(a, me))));
}

const me = await currentUser().catch(() => null);
// Who is not signed in finds the accounts that wait on the page of
// signing in ("Войти как:").
if (!me) location.replace('/');
renderChrome(me);

if (me) {
  $('add').addEventListener('click', () => addAccount(me));
  showAccounts(() => draw(me));
}
