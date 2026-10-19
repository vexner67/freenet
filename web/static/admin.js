// "Админ-панель", openvk's /admin: "Сводка" (/admin), "Пользователи"
// (/admin/users: the verified pages, or those ?q= finds, with the switch
// of "Верификация" each), "Ваучеры" (/admin/vouchers) and the form of a
// new one (/admin/vouchers/new). The page is its own, AUI as openvk's:
// none of the site's chrome. Only the administrators of the site
// (ADMIN_EMAILS of the user service) get in.

import { api, currentUser, idle, started } from './api.js';
import { adminOverview, errorText, voucherCode, voucherStatus, voucherUsages } from './labels.js';

const $ = (id) => document.getElementById(id);

function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  Object.assign(node, props);
  node.append(...children);
  return node;
}

const root = document.documentElement;
const path = location.pathname.replace(/\/+$/, '') || '/admin';

function store(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // The choice lasts until the page is left.
  }
}

// The toggle in the header is the panel's dark mode, as openvk's.
const toggle = $('switch-theme');
const showTheme = () => toggle.setAttribute('aria-checked', String(root.classList.contains('aui-theme-dark')));
showTheme();
toggle.addEventListener('click', () => {
  const dark = root.classList.toggle('aui-theme-dark');
  store('freenet.admintheme', dark ? 'dark' : 'light');
  showTheme();
});

// The button under the sidebar folds it to icons and back.
$('sidebar-toggle').addEventListener('click', () => {
  const folded = root.classList.toggle('aui-collapsed');
  store('freenet.adminsidebar', folded ? 'folded' : 'open');
  $('sidebar-toggle').title = folded ? 'Развернуть боковую панель' : 'Свернуть боковую панель';
});
if (root.classList.contains('aui-collapsed')) $('sidebar-toggle').title = 'Развернуть боковую панель';

// The link of the page is the active one; the form of a voucher is under
// "Ваучеры".
for (const link of document.querySelectorAll('.aui-nav-item')) {
  const href = link.getAttribute('href');
  link.classList.toggle('active', href === path || (href !== '/admin' && path.startsWith(href + '/')));
}

const error = $('error');

function showError(e) {
  error.textContent = typeof e === 'string' ? e : errorText(e);
  error.hidden = false;
}

const fullName = (user) => [user.name, user.surname].filter(Boolean).join(' ');

// number is written as openvk's: "1 234".
const number = (n) => n.toLocaleString('ru-RU');

// summary is "Сводка": what the site has, as the cards of openvk's.
async function summary(counts) {
  const answer = await counts;
  const words = adminOverview(answer);

  for (const key of ['users', 'online', 'verified', 'clubs', 'posts']) {
    $('count-' + key).textContent = number(answer[key]);
    $(key + '-words').textContent = words[key];
  }
  $('summary').hidden = false;
}

// checkmark is the site's mark of a confirmed page, as by its names.
const checkmark = () => el('img', {
  className: 'name-checkmark', src: '/static/icons/checkmark.svg', alt: '', width: 13, height: 11,
  title: 'Подтверждённая страница',
});

// users is "Пользователи": the pages the query finds or, without one, the
// verified pages, each with the switch of openvk's "Верификация".
function users(query) {
  let cursor = '';
  const search = (after) => api.search(query, after, query ? {} : { verified: 'true' });

  function row(user) {
    const mark = user.verified ? checkmark() : '';
    const name = fullName(user);
    const toggle = el('button', {
      className: 'aui-toggle', type: 'button', title: 'Подтверждённая страница',
    }, el('span', { className: 'aui-toggle-knob' }));
    toggle.setAttribute('role', 'switch');
    toggle.setAttribute('aria-label', `Верификация: ${name}`);
    toggle.setAttribute('aria-checked', String(Boolean(user.verified)));

    const person = el('div', { className: 'admin-person' },
      el('img', { className: 'admin-avatar', src: user.avatar?.small ?? '/static/avatar.svg', alt: '' }),
      el('span', {}, el('a', { href: '/' + user.nickname, textContent: name }), mark));

    toggle.addEventListener('click', async () => {
      const verified = toggle.getAttribute('aria-checked') !== 'true';
      error.hidden = true;
      toggle.disabled = true;
      try {
        await api.setVerified(user.nickname, verified);
        toggle.setAttribute('aria-checked', String(verified));
        const shown = person.querySelector('.name-checkmark');
        if (verified && !shown) person.lastChild.append(checkmark());
        if (!verified) shown?.remove();
      } catch (e) {
        showError(e);
      } finally {
        toggle.disabled = false;
      }
    });

    return el('tr', {},
      el('td', {}, person),
      el('td', {}, el('span', { className: 'admin-code', textContent: '@' + user.nickname })),
      el('td', {}, toggle),
    );
  }

  async function more(page) {
    error.hidden = true;
    const button = $('more-users');
    button.disabled = true;

    try {
      const answer = await page;
      $('user-rows').append(...answer.users.map(row));
      cursor = answer.nextCursor;
      button.hidden = !cursor;

      const none = !$('user-rows').children.length;
      $('user-table').hidden = none;
      $('no-users').hidden = !none;
    } catch (e) {
      showError(e);
    } finally {
      button.disabled = false;
    }
  }

  $('user-query').value = query;
  $('users-heading').textContent = query ? 'Результаты поиска' : 'Подтверждённые страницы';
  $('no-users').textContent = query ? 'Никого не найдено.' : 'Подтверждённых страниц пока нет. Найдите человека поиском.';
  $('more-users').addEventListener('click', () => more(search(cursor)));

  return { more, first: () => search('') };
}

// vouchers is the list of openvk's "Ваучеры", newest first.
function vouchers() {
  let cursor = '';

  function row(voucher) {
    const ended = voucherStatus(voucher) === 'закончился';
    const remove = el('button', { className: 'aui-button aui-button-compact', type: 'button', textContent: 'Удалить' });
    const tr = el('tr', {},
      el('td', {}, el('span', { className: 'admin-code', textContent: voucherCode(voucher.code) })),
      el('td', { textContent: number(voucher.votes) }),
      el('td', { textContent: number(voucher.rating) }),
      el('td', { textContent: voucherUsages(voucher.usagesLeft) }),
      el('td', { textContent: number(voucher.activations) }),
      el('td', {}, el('span', {
        className: 'aui-lozenge ' + (ended ? 'aui-lozenge-removed' : 'aui-lozenge-success'),
        textContent: voucherStatus(voucher),
      })),
      el('td', {}, remove),
    );

    remove.addEventListener('click', async () => {
      if (!confirm(`Удалить ваучер ${voucherCode(voucher.code)}? Полученное по нему останется у тех, кто им воспользовался.`)) return;

      error.hidden = true;
      remove.disabled = true;
      try {
        await api.deleteVoucher(voucher.id);
        tr.remove();
        showEmpty();
      } catch (e) {
        remove.disabled = false;
        showError(e);
      }
    });

    return tr;
  }

  function showEmpty() {
    const none = !$('voucher-rows').children.length;
    $('voucher-table').hidden = none;
    $('no-vouchers').hidden = !none;
  }

  async function more(page) {
    error.hidden = true;
    const button = $('more-vouchers');
    button.disabled = true;

    try {
      const answer = await page;
      $('voucher-rows').append(...answer.vouchers.map(row));
      cursor = answer.nextCursor;
      button.hidden = !cursor;
      showEmpty();
    } catch (e) {
      showError(e);
    } finally {
      button.disabled = false;
    }
  }

  $('more-vouchers').addEventListener('click', () => more(api.vouchers(cursor)));
  $('vouchers').hidden = false;

  return more;
}

// newVoucher is the form of a voucher, openvk's /admin/vouchers/id0.
function newVoucher() {
  const form = $('voucher-create');

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    $('created').hidden = true;

    const submit = form.querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const voucher = await api.createVoucher({
        code: $('voucher-code').value.trim(),
        votes: Number($('voucher-votes').value),
        rating: Number($('voucher-rating').value),
        usages: Number($('voucher-usages').value),
      });

      $('created-code').textContent = voucherCode(voucher.code);
      $('created').hidden = false;
      $('voucher-code').value = '';
    } catch (e) {
      showError(e);
    } finally {
      submit.disabled = false;
    }
  });

  $('voucher-new').hidden = false;
  $('voucher-code').focus();
}

// The reads of the page go with /me in one round trip; a guest or one who
// is not an administrator gets nothing from them.
const query = new URLSearchParams(location.search).get('q')?.trim() ?? '';
const usersPage = path === '/admin/users' ? users(query) : null;

const reads = {
  '/admin': () => started(api.about()),
  '/admin/users': () => started(usersPage.first()),
  '/admin/vouchers': () => started(api.vouchers()),
}[path]?.();

const me = await currentUser().catch(() => null);

if (!me) {
  location.replace('/');
} else {
  if (!me.admin) {
    showError('Админ-панель доступна только администраторам сайта.');
  } else if (path === '/admin') {
    await summary(reads).catch(showError);
  } else if (path === '/admin/users') {
    $('users').hidden = false;
    await usersPage.more(reads);
  } else if (path === '/admin/vouchers') {
    await vouchers()(reads);
  } else if (path === '/admin/vouchers/new') {
    newVoucher();
  } else {
    showError('Такой страницы нет.');
  }

  idle().then(() => root.classList.remove('pending'));
}
