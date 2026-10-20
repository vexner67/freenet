// "Админ-панель", openvk's /admin: "Сводка" (/admin), "Пользователи"
// (/admin/users: the verified pages, or those ?q= finds, with the switch
// of "Верификация", the orange label and "Заблокировать"
// each), "Блокировки" (/admin/bans: the pages blocked now), "Группы" (/admin/clubs: the same of groups),
// "Ваучеры" (/admin/vouchers), the form of a new one
// (/admin/vouchers/new), "Подарки" (/admin/gifts: what each gift costs)
// "Оформление" (/admin/decor: the New Year decoration of the site),
// "Казино" (/admin/casino: the price of a spin, the prizes and their odds,
// and the limits and the return of the bets)
// and "Реклама" (/admin/ads: the ads of the global feed, the form of a new
// one at /admin/ads/new and of one at /admin/ads/<id>). The page is its own, AUI as openvk's:
// none of the site's chrome. Only the administrators of the site
// (ADMIN_EMAILS of the user service) get in.

import { api, currentUser, idle, started } from './api.js';
import { accusativeName, foreignAgentText, adminOverview, adRate, betChance, betMultiplier, betOdds, wagersCount, casinoChance, casinoChances, casinoPayout, casinoReturn, spinsCount, votesCount, peopleCount, banEnd, banUntil, errorText, genitiveName, giftCollections, giftCosts, giftImage, giftNames, giftPrice, voteRate, voucherCode, voucherStatus, voucherUsages } from './labels.js';
import { tr } from './i18n.js';
import { remember } from './newyear.js';

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
  $('sidebar-toggle').title = folded ? tr('Развернуть боковую панель') : tr('Свернуть боковую панель');
});
if (root.classList.contains('aui-collapsed')) $('sidebar-toggle').title = tr('Развернуть боковую панель');

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
  title: tr('Подтверждённая страница'),
});

// The pages that wear the checkmark, "Пользователи" and "Группы": each a
// table of what the query finds or, without one, of the verified, with
// the switch of openvk's "Верификация".
const verifiable = {
  users: {
    one: 'user',
    search: (query, after) => api.search(query, after, query ? {} : { verified: 'true' }),
    items: (answer) => answer.users,
    name: fullName,
    path: (user) => '/' + user.nickname,
    address: (user) => '@' + user.nickname,
    avatar: (user) => user.avatar?.small ?? '/static/avatar.svg',
    verify: (user, verified) => api.setVerified(user.nickname, verified),
    all: tr('Подтверждённые страницы'),
    none: tr('Никого не найдено.'),
    noneVerified: tr('Подтверждённых страниц пока нет. Найдите человека поиском.'),
    alertCell,
    banCell,
  },
  clubs: {
    one: 'club',
    search: (query, after) => api.searchClubs(query, after, query ? {} : { verified: 'true' }),
    items: (answer) => answer.clubs,
    name: (club) => club.name,
    path: (club) => '/club/' + encodeURIComponent(club.address || club.id),
    address: (club) => '/club/' + (club.address || club.id),
    avatar: (club) => club.avatar?.small ?? '/static/club.svg',
    verify: (club, verified) => api.setClubVerified(club.address || club.id, verified),
    all: tr('Подтверждённые группы'),
    none: tr('Ни одной группы не нашлось.'),
    noneVerified: tr('Подтверждённых групп пока нет. Найдите группу поиском.'),
  },
};

// alertCell is "Метка" of a person: the words of the orange label above
// the name (openvk's "user-alert") or "нет", and "Изменить", which opens
// the dialog.
function alertCell(user) {
  const cell = el('div', { className: 'admin-alert' });

  const draw = () => cell.replaceChildren(
    el('span', {
      className: 'admin-alert-text' + (user.alert ? '' : ' empty'),
      textContent: user.alert || tr('нет'), title: user.alert || '',
    }),
    el('button', {
      className: 'aui-button aui-button-compact', type: 'button', textContent: tr('Изменить'),
      onclick: () => openAlert(user, draw),
    }),
  );

  draw();

  return cell;
}

// openAlert asks the words of the label of user, offers those of a foreign
// agent, and draws the cell anew once saved or taken.
function openAlert(user, done) {
  const dialog = $('alert-dialog');
  const form = $('alert-form');
  const text = $('alert-text');
  const alertError = $('alert-error');

  $('alert-about').textContent = tr('Метка встанет оранжевой плашкой над именем на странице {0} (@{1}). Её увидят все, кто откроет страницу.', genitiveName(user), user.nickname);
  form.reset();
  text.value = user.alert || '';
  $('alert-remove').hidden = !user.alert;
  alertError.hidden = true;
  dialog.hidden = false;
  text.focus();

  const close = () => {
    dialog.hidden = true;
    form.onsubmit = null;
    document.removeEventListener('keydown', onKey);
  };
  const onKey = (e) => {
    if (e.key === 'Escape') close();
  };
  document.addEventListener('keydown', onKey);
  $('alert-cancel').onclick = close;
  dialog.onclick = (e) => {
    if (e.target === dialog) close();
  };
  $('alert-agent').onclick = (e) => {
    e.preventDefault();
    text.value = foreignAgentText(user);
    text.focus();
  };

  const save = async (alert, button) => {
    alertError.hidden = true;
    button.disabled = true;
    try {
      await api.setAlert(user.nickname, alert);
      user.alert = alert;
      close();
      done();
    } catch (err) {
      alertError.textContent = errorText(err);
      alertError.hidden = false;
    } finally {
      button.disabled = false;
    }
  };

  // The label is one line: the line breaks of the field become spaces.
  form.onsubmit = (e) => {
    e.preventDefault();
    save(text.value.replace(/\s*[\r\n]+\s*/g, ' ').trim(), form.querySelector('[type=submit]'));
  };
  $('alert-remove').onclick = () => save('', $('alert-remove'));
}

// banCell is "Блокировка" of a person: "заблокирован", until when and
// "Разблокировать", or "Заблокировать", which opens the dialog.
function banCell(user) {
  const cell = el('div', { className: 'admin-ban' });

  const draw = () => {
    if (!user.ban) {
      cell.replaceChildren(el('button', {
        className: 'aui-button aui-button-compact', type: 'button', textContent: tr('Заблокировать'),
        onclick: () => openBan(user, draw),
      }));
      return;
    }

    const lift = el('button', { className: 'aui-button aui-button-compact', type: 'button', textContent: tr('Разблокировать') });
    lift.addEventListener('click', async () => {
      error.hidden = true;
      lift.disabled = true;
      try {
        await api.unban(user.nickname);
        user.ban = null;
        draw();
      } catch (e) {
        showError(e);
        lift.disabled = false;
      }
    });

    cell.replaceChildren(
      el('span', { className: 'aui-lozenge aui-lozenge-removed', textContent: tr('заблокирован') }),
      el('span', { className: 'admin-ban-until', textContent: banEnd(user.ban), title: user.ban.reason }),
      lift,
    );
  };

  draw();

  return cell;
}

// openBan asks why and for how long to block user, openvk's "Забанить …",
// and draws the cell anew once blocked.
function openBan(user, done) {
  const dialog = $('ban-dialog');
  const form = $('ban-form');
  const banError = $('ban-error');

  $('ban-title').textContent = tr('Заблокировать ') + accusativeName(user);
  $('ban-about').textContent = tr('Вы собираетесь заблокировать страницу {0} (@{1}). ', genitiveName(user), user.nickname)
    + tr('Пока блокировка действует, владелец сможет только прочитать её причину и выйти, а страница покажет другим лишь имя, фото и причину.');
  form.reset();
  $('ban-date-group').hidden = true;
  banError.hidden = true;
  dialog.hidden = false;
  $('ban-reason').focus();

  const close = () => {
    dialog.hidden = true;
    form.onsubmit = null;
    document.removeEventListener('keydown', onKey);
  };
  const onKey = (e) => {
    if (e.key === 'Escape') close();
  };
  document.addEventListener('keydown', onKey);
  $('ban-cancel').onclick = close;
  dialog.onclick = (e) => {
    if (e.target === dialog) close();
  };
  $('ban-term').onchange = () => {
    $('ban-date-group').hidden = $('ban-term').value !== 'date';
  };

  form.onsubmit = async (e) => {
    e.preventDefault();
    banError.hidden = true;

    const term = $('ban-term').value;
    const until = banUntil(term, new Date(), $('ban-date').value);
    if (term === 'date' && !until) {
      banError.textContent = tr('Укажите, до какого времени заблокировать.');
      banError.hidden = false;
      return;
    }

    const submit = form.querySelector('[type=submit]');
    submit.disabled = true;
    try {
      const reason = $('ban-reason').value.trim();
      await api.ban(user.nickname, reason, until);
      user.ban = { reason, bannedAt: new Date().toISOString(), until: until || null };
      close();
      done();
    } catch (err) {
      banError.textContent = errorText(err);
      banError.hidden = false;
    } finally {
      submit.disabled = false;
    }
  };
}

// verifications is the page of a kind of verifiable, for query; bans:
// without a query it lists the pages blocked now ("Блокировки").
function verifications(kind, query, bans = false) {
  const k = bans ? {
    ...verifiable[kind],
    search: (q, after) => api.search(q, after, q ? {} : { banned: 'true' }),
    all: tr('Заблокированные пользователи'),
    noneVerified: tr('Заблокированных пользователей нет.'),
  } : verifiable[kind];
  let cursor = '';

  function row(page) {
    const mark = page.verified ? checkmark() : '';
    const name = k.name(page);
    const toggle = el('button', {
      className: 'aui-toggle', type: 'button', title: tr('Подтверждённая страница'),
    }, el('span', { className: 'aui-toggle-knob' }));
    toggle.setAttribute('role', 'switch');
    toggle.setAttribute('aria-label', tr('Верификация: {0}', name));
    toggle.setAttribute('aria-checked', String(Boolean(page.verified)));

    const person = el('div', { className: 'admin-person' },
      el('img', { className: kind === 'clubs' ? 'admin-avatar admin-club' : 'admin-avatar', src: k.avatar(page), alt: '' }),
      el('span', {}, el('a', { href: k.path(page), textContent: name }), mark));

    toggle.addEventListener('click', async () => {
      const verified = toggle.getAttribute('aria-checked') !== 'true';
      error.hidden = true;
      toggle.disabled = true;
      try {
        await k.verify(page, verified);
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

    return el('tr', {}, ...[
      el('td', {}, person),
      el('td', {}, el('span', { className: 'admin-code', textContent: k.address(page) })),
      el('td', {}, toggle),
      k.alertCell && el('td', {}, k.alertCell(page)),
      k.banCell && el('td', {}, k.banCell(page)),
    ].filter(Boolean));
  }

  async function more(answered) {
    error.hidden = true;
    const button = $('more-' + kind);
    button.disabled = true;

    try {
      const answer = await answered;
      $(k.one + '-rows').append(...k.items(answer).map(row));
      cursor = answer.nextCursor;
      button.hidden = !cursor;

      const none = !$(k.one + '-rows').children.length;
      $(k.one + '-table').hidden = none;
      $('no-' + kind).hidden = !none;
    } catch (e) {
      showError(e);
    } finally {
      button.disabled = false;
    }
  }

  $(k.one + '-query').value = query;
  $(kind + '-heading').textContent = query ? tr('Результаты поиска') : k.all;
  $('no-' + kind).textContent = query ? k.none : k.noneVerified;
  $('more-' + kind).addEventListener('click', () => more(k.search(query, cursor)));

  return { more, first: () => k.search(query, '') };
}

// vouchers is the list of openvk's "Ваучеры", newest first.
function vouchers() {
  let cursor = '';

  function row(voucher) {
    const ended = voucher.usagesLeft === 0;
    const remove = el('button', { className: 'aui-button aui-button-compact', type: 'button', textContent: tr('Удалить') });
    const line = el('tr', {},
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
      if (!confirm(tr('Удалить ваучер {0}? Полученное по нему останется у тех, кто им воспользовался.', voucherCode(voucher.code)))) return;

      error.hidden = true;
      remove.disabled = true;
      try {
        await api.deleteVoucher(voucher.id);
        line.remove();
        showEmpty();
      } catch (e) {
        remove.disabled = false;
        showError(e);
      }
    });

    return line;
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

// gifts is "Подарки": every gift of the catalogue with the field of its
// price, saved row by row.
async function gifts({ catalog, rate }) {
  const { collections } = await catalog;

  function row(collection, { gift, price }) {
    const name = giftNames[gift] ?? gift;
    const field = el('input', { className: 'text', type: 'number', min: 0, max: 1000000, value: String(price), required: true });
    field.setAttribute('aria-label', tr('Цена: {0}', name));
    const save = el('button', { className: 'aui-button', type: 'submit', textContent: tr('Сохранить') });
    const form = el('form', { className: 'aui admin-price' }, field, save);

    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      error.hidden = true;
      $('priced').hidden = true;
      save.disabled = true;

      try {
        const priced = Number(field.value);
        await api.setGiftPrice(gift, priced);
        $('priced').textContent = `${name}: ${giftPrice(priced).toLowerCase()}.`;
        $('priced').hidden = false;
      } catch (e) {
        showError(e);
      } finally {
        save.disabled = false;
      }
    });

    return el('tr', {},
      el('td', {}, el('div', { className: 'admin-gift' }, el('img', { src: giftImage(gift), alt: '' }), name)),
      el('td', { textContent: giftCollections[collection.id] ?? collection.id }),
      el('td', {}, form),
    );
  }

  $('gift-rows').replaceChildren(...collections.flatMap((c) => giftCosts(c).map((g) => row(c, g))));
  showRate(await rate);
  $('gifts').hidden = false;
}

// showRate is the form of what a vote is worth, in rubles with kopecks.
function showRate({ kopecks }) {
  const field = $('rate-rubles');
  const save = $('rate-form').querySelector('button');

  field.value = (kopecks / 100).toFixed(2);
  $('rate-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    $('priced').hidden = true;
    save.disabled = true;

    try {
      const set = Math.round(Number(field.value) * 100);
      await api.setVoteRate(set);
      $('priced').textContent = tr('Курс: {0}.', voteRate(set));
      $('priced').hidden = false;
    } catch (e) {
      showError(e);
    } finally {
      save.disabled = false;
    }
  });
}

// decor is "Оформление": whether the site wears the New Year decoration
// — by the calendar, always or never — and whether it does now.
async function decor(answered) {
  const field = $('decor-mode');
  const save = $('decor-form').querySelector('button');

  const show = ({ mode, festive }) => {
    field.value = mode;
    $('decor-now').textContent = festive ? tr('Сейчас сайт украшен к Новому году.') : tr('Сейчас сайт не украшен.');
    // This browser sees the change at once, the others within minutes.
    remember(festive);
  };
  show(await answered);

  $('decor-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    $('decor-saved').hidden = true;
    save.disabled = true;

    try {
      await api.setDecor(field.value);
      show(await api.decor());
      $('decor-saved').textContent = tr('Изменения сохранены');
      $('decor-saved').hidden = false;
    } catch (e) {
      showError(e);
    } finally {
      save.disabled = false;
    }
  });

  $('decor').hidden = false;
}

// casino is "Казино": the price of a spin and the prizes with their
// weights, the chance of each and the share of the votes the casino gives
// back by them as they are typed, and how it has done in fact.
async function casino(answered) {
  const { price, prizes, spins, spent, won, bets, wagers } = await answered;
  const rows = $('casino-rows');
  const priceField = $('casino-price');
  const save = $('casino-form').querySelector('button[type=submit]');

  const typed = () => [...rows.children].map((row) => ({
    votes: Number(row.parts.votes.value),
    weight: Number(row.parts.weight.value),
  }));

  // recount shows the chances and the return of what is typed; the
  // casino has 2 to 12 prizes.
  function recount() {
    const prizes = typed();
    const chances = casinoChances(prizes);

    [...rows.children].forEach((row, i) => {
      row.parts.chance.textContent = casinoChance(chances[i]);
      row.parts.remove.disabled = rows.children.length <= 2;
    });
    $('casino-add').disabled = rows.children.length >= 12;
    $('casino-return').textContent = tr('Казино возвращает в среднем {0} потраченных голосов.', casinoReturn(Number(priceField.value), prizes));
  }

  function row(prize) {
    const votes = el('input', { className: 'text admin-casino-field', type: 'number', min: 0, max: 1000000, value: String(prize.votes), required: true });
    const weight = el('input', { className: 'text admin-casino-field', type: 'number', min: 0, max: 1000000, value: String(prize.weight), required: true });
    votes.setAttribute('aria-label', tr('Приз, голосов'));
    weight.setAttribute('aria-label', tr('Вес'));
    const chance = el('td');
    const remove = el('button', { className: 'aui-button aui-button-compact', type: 'button', textContent: tr('Удалить') });
    const line = el('tr', {}, el('td', {}, votes), el('td', {}, weight), chance, el('td', {}, remove));

    line.parts = { votes, weight, chance, remove };
    remove.addEventListener('click', () => {
      line.remove();
      recount();
    });

    return line;
  }

  $('casino-stats').textContent = spins > 0
    ? tr('Сыграно {0}: потрачено {1}, выиграно {2} — {3} потраченного.', spinsCount(spins), votesCount(spent), votesCount(won), casinoPayout({ spent, won }))
    : tr('Ещё никто не играл.');
  priceField.value = String(price);
  rows.replaceChildren(...prizes.map(row));
  recount();

  $('casino-form').addEventListener('input', recount);
  $('casino-add').addEventListener('click', () => {
    rows.append(row({ votes: 0, weight: 1 }));
    recount();
  });

  $('casino-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    $('casino-saved').hidden = true;
    save.disabled = true;

    try {
      await api.setCasino(Number(priceField.value), typed());
      $('casino-saved').textContent = tr('Изменения сохранены');
      $('casino-saved').hidden = false;
    } catch (e) {
      showError(e);
    } finally {
      save.disabled = false;
    }
  });

  betSettings(bets, wagers);
  $('casino').hidden = false;
}

// betSettings is "Ставка" of "Казино": the limits of a bet and the return,
// the chance of each multiplier by it as it is typed, and how the bets
// have done in fact.
function betSettings(bets, wagers) {
  const least = $('bets-least');
  const most = $('bets-most');
  const ret = $('bets-return');
  const save = $('bets-form').querySelector('button[type=submit]');

  function recount() {
    const percent = Number(ret.value);

    $('bets-chances').textContent = percent >= 1 && percent <= 100
      ? tr('Шансы: {0}.', bets.multipliers.map((m) => `${betMultiplier(m)} — ${betChance(betOdds(percent, m))}`).join(', '))
      : '';
  }

  $('bets-stats').textContent = wagers.count > 0
    ? tr('Сделано {0}: поставлено {1}, выиграно {2} — {3} поставленного.', wagersCount(wagers.count), votesCount(wagers.spent), votesCount(wagers.won), casinoPayout(wagers))
    : tr('Ещё никто не ставил.');
  least.value = String(bets.least);
  most.value = String(bets.most);
  ret.value = String(bets.return);
  recount();

  ret.addEventListener('input', recount);
  $('bets-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    $('bets-saved').hidden = true;
    save.disabled = true;

    try {
      await api.setCasinoBets(Number(least.value), Number(most.value), Number(ret.value));
      $('bets-saved').textContent = tr('Изменения сохранены');
      $('bets-saved').hidden = false;
    } catch (e) {
      showError(e);
    } finally {
      save.disabled = false;
    }
  });
}

// ads is "Реклама": every ad, shown or not, with how it does (views and
// clicks, by how many people, and the share of views followed), the
// switch of "Показывать" and "Изменить" and "Удалить" each.
async function ads(answered) {
  // statCell is a count with how many people it was, under it.
  const statCell = (count, people) => el('td', { className: 'admin-stat' },
    number(count), el('div', { className: 'admin-ad-site', textContent: peopleCount(people) }));

  function row(ad) {
    const stats = ad.stats ?? { views: 0, viewers: 0, clicks: 0, clickers: 0 };
    const toggle = el('button', {
      className: 'aui-toggle', type: 'button', title: tr('Показывать в ленте'),
    }, el('span', { className: 'aui-toggle-knob' }));
    toggle.setAttribute('role', 'switch');
    toggle.setAttribute('aria-label', tr('Показывать: {0}', ad.title));
    toggle.setAttribute('aria-checked', String(ad.active));

    toggle.addEventListener('click', async () => {
      const active = toggle.getAttribute('aria-checked') !== 'true';
      error.hidden = true;
      toggle.disabled = true;
      try {
        await api.editAd(ad.id, { active });
        toggle.setAttribute('aria-checked', String(active));
      } catch (e) {
        showError(e);
      } finally {
        toggle.disabled = false;
      }
    });

    const remove = el('button', { className: 'aui-button aui-button-compact', type: 'button', textContent: tr('Удалить') });
    const line = el('tr', {},
      el('td', {}, el('div', { className: 'admin-ad' },
        ad.photo ? el('img', { className: 'admin-ad-cover', src: ad.photo.small, alt: '' }) : '',
        el('div', {},
          el('a', { href: ad.url, target: '_blank', rel: 'noopener noreferrer', textContent: ad.title }),
          el('div', { className: 'admin-ad-site', textContent: ad.site }),
        ),
      )),
      el('td', { className: 'admin-ad-text', textContent: ad.text }),
      statCell(stats.views, stats.viewers),
      statCell(stats.clicks, stats.clickers),
      el('td', { className: 'admin-stat', textContent: adRate(stats) }),
      el('td', {}, toggle),
      el('td', {}, el('div', { className: 'admin-actions' },
        el('a', { className: 'aui-button aui-button-compact', href: '/admin/ads/' + encodeURIComponent(ad.id), textContent: tr('Изменить') }),
        remove,
      )),
    );

    remove.addEventListener('click', async () => {
      if (!confirm(tr('Удалить объявление «{0}»?', ad.title))) return;

      error.hidden = true;
      remove.disabled = true;
      try {
        await api.deleteAd(ad.id);
        line.remove();
        showEmpty();
      } catch (e) {
        remove.disabled = false;
        showError(e);
      }
    });

    return line;
  }

  function showEmpty() {
    const none = !$('ad-rows').children.length;
    $('ad-table').hidden = none;
    $('no-ads').hidden = !none;
  }

  $('ads').hidden = false;
  const answer = await answered;
  $('ad-rows').replaceChildren(...answer.ads.map(row));
  showEmpty();
}

// adForm is the form of an ad: a new one without id, else the one of id
// among answered. The cover is uploaded as soon as it is chosen.
async function adForm(id, answered) {
  const form = $('ad-form');
  const submit = form.querySelector('button[type="submit"]');
  let ad = null;
  // cover is the cover the form shows: { id, small } or null; photo is
  // what to send of it, undefined when it is the ad's own.
  let cover = null;
  let photo;

  const showCover = () => {
    $('ad-cover').hidden = !cover;
    $('ad-cover-image').src = cover?.small ?? '';
  };

  if (id) {
    ad = (await answered).ads.find((a) => a.id === id);
    if (!ad) {
      showError(tr('Такого объявления нет.'));
      return;
    }

    $('ad-heading').textContent = tr('Объявление «{0}»', ad.title);
    $('ad-url').value = ad.url;
    $('ad-title').value = ad.title;
    $('ad-text').value = ad.text;
    $('ad-active').checked = ad.active;
    cover = ad.photo;
  } else {
    photo = '';
  }
  showCover();

  $('ad-file').addEventListener('change', async () => {
    const file = $('ad-file').files[0];
    if (!file) return;

    error.hidden = true;
    submit.disabled = true;
    try {
      cover = await api.uploadPhoto(file);
      photo = cover.id;
      showCover();
    } catch (e) {
      showError(e);
    } finally {
      $('ad-file').value = '';
      submit.disabled = false;
    }
  });

  $('ad-cover-remove').addEventListener('click', () => {
    cover = null;
    photo = '';
    showCover();
  });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    $('ad-saved').hidden = true;
    submit.disabled = true;

    const fields = {
      url: $('ad-url').value.trim(),
      title: $('ad-title').value.trim(),
      text: $('ad-text').value.trim(),
      active: $('ad-active').checked,
    };

    try {
      if (ad) {
        await api.editAd(ad.id, photo === undefined ? fields : { ...fields, photo });
      } else {
        await api.createAd({ ...fields, photo });
      }
      location.assign('/admin/ads');
    } catch (e) {
      showError(e);
      submit.disabled = false;
    }
  });

  $('ad-edit').hidden = false;
  $('ad-url').focus();
}

// The reads of the page go with /me in one round trip; a guest or one who
// is not an administrator gets nothing from them.
const query = new URLSearchParams(location.search).get('q')?.trim() ?? '';
const listed = { '/admin/users': 'users', '/admin/bans': 'users', '/admin/clubs': 'clubs' }[path];
const listPage = listed ? verifications(listed, query, path === '/admin/bans') : null;

const adID = path.match(/^\/admin\/ads\/([0-9a-f-]{36})$/)?.[1] ?? '';

const reads = {
  '/admin': () => started(api.about()),
  '/admin/users': () => started(listPage.first()),
  '/admin/bans': () => started(listPage.first()),
  '/admin/clubs': () => started(listPage.first()),
  '/admin/vouchers': () => started(api.vouchers()),
  '/admin/gifts': () => ({ catalog: started(api.giftCatalog()), rate: started(api.voteRate()) }),
  '/admin/ads': () => started(api.ads(true)),
  '/admin/decor': () => started(api.decor()),
  '/admin/casino': () => started(api.casinoSettings()),
}[adID ? '/admin/ads' : path]?.();

const me = await currentUser().catch(() => null);

if (!me) {
  location.replace('/');
} else {
  if (!me.admin) {
    showError(tr('Админ-панель доступна только администраторам сайта.'));
  } else if (path === '/admin') {
    await summary(reads).catch(showError);
  } else if (listed) {
    if (path === '/admin/bans') {
      $('users-title').textContent = tr('Блокировки');
      $('user-search').action = '/admin/bans';
    }
    $(listed).hidden = false;
    await listPage.more(reads);
  } else if (path === '/admin/vouchers') {
    await vouchers()(reads);
  } else if (path === '/admin/vouchers/new') {
    newVoucher();
  } else if (path === '/admin/gifts') {
    await gifts(reads).catch(showError);
  } else if (path === '/admin/decor') {
    await decor(reads).catch(showError);
  } else if (path === '/admin/casino') {
    await casino(reads).catch(showError);
  } else if (path === '/admin/ads') {
    await ads(reads).catch(showError);
  } else if (path === '/admin/ads/new' || adID) {
    await adForm(adID, reads).catch(showError);
  } else {
    showError(tr('Такой страницы нет.'));
  }

  idle().then(() => root.classList.remove('pending'));
}
