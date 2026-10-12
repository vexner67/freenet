// openvk's "Мои Настройки" at /settings: "Основное" (the e-mail, the
// page address and "удалить свою страницу"), "Безопасность" (?act=security:
// where you are signed in) and "Приватность" (?act=privacy:
// who sees the page and its parts and who writes to you) and "Внешний вид"
// (?act=interface: the style, the photos and the posts, the clock, where
// the site opens and what the left menu shows); "Чёрный список" is a page of its own.
// "Голоса" (?act=finance: the balance and "передать голоса";
// ?act=finance.top-up: "Есть ваучер?") and, for the administrators of the
// site, "Ваучеры" (?act=vouchers: the vouchers and the form that makes one).

import { api, currentUser } from './api.js';
import { deviceName, formatDateTime, sessionsCount, plural, votesCount, voucherCode, voucherGave, voucherStatus, voucherUsages, audiences, avatarStyles, mainPages, menuItems, nsfwContents, pageReads, postViews, onlineReads, ratings, themes, timezones, wallWrites, messagesWrites, chatsInvites, profileTypes } from './labels.js';
import { $, applyInterface, deviceTheme, el, fullName, pagePath, setDeviceTheme, profilePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { openDeactivate } from './deactivate.js';

const error = $('error');
const tabs = {
  security: initSecurity, privacy: initPrivacy, interface: initInterface,
  finance: initFinance, 'finance.top-up': initTopUp, vouchers: initVouchers,
};
const asked = new URLSearchParams(location.search).get('act');

function fill(select, options, value) {
  select.replaceChildren(...options.map(([v, text]) => el('option', { value: v, textContent: text })));
  select.value = value;
}

// onSubmit saves a form with its button disabled, as openvk: "Изменения
// сохранены" above it after, errors in their place.
function onSubmit(form, save) {
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;

    try {
      await save();
      $('notice').hidden = false;
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
      $('notice').scrollIntoView({ block: 'nearest' });
    }
  });
}

// initMain: the page address is the nickname, saved alone, so a profile
// edited in another tab since this page was opened stays as edited.
function initMain(me) {
  $('email').textContent = me.email;
  $('page-id').textContent = me.id;
  $('nickname').value = me.nickname;

  onSubmit($('address'), async () => {
    const updated = await api.updateNickname($('nickname').value.trim());
    // "Моя Страница" in the header and the menu leads to the new address.
    for (const link of document.querySelectorAll(`a[href="${profilePath(me)}"]`)) link.href = profilePath(updated);
    Object.assign(me, updated);
    $('nickname').value = updated.nickname;
  });

  $('delete-link').addEventListener('click', () => openDeactivate(me));
}

const reads = {
  'page-read': 'pageRead', 'info-read': 'infoRead', 'online-read': 'onlineRead',
  'clubs-read': 'clubsRead', 'photos-read': 'photosRead', 'friends-read': 'friendsRead',
  'notes-read': 'notesRead', 'gifts-read': 'giftsRead',
};
const readOptions = { 'page-read': pageReads, 'online-read': onlineReads };
const shows = { 'show-birthday': 'showBirthday', 'show-relationship': 'showRelationship', 'show-worldview': 'showWorldview', 'show-political-views': 'showPoliticalViews' };

function initPrivacy(me) {
  for (const [id, key] of Object.entries(reads)) fill($(id), readOptions[id] ?? audiences, me.privacy[key]);
  for (const [id, key] of Object.entries(shows)) $(id).checked = me.privacy[key];
  fill($('wall-write'), wallWrites, me.privacy.wallWrite);
  fill($('messages-write'), messagesWrites, me.privacy.messagesWrite);
  fill($('chats-invite'), chatsInvites, me.privacy.chatsInvite);
  fill($('profile-type'), profileTypes, me.privacy.closed ? 'closed' : 'open');

  onSubmit($('privacy'), async () => {
    const privacy = { wallWrite: $('wall-write').value, messagesWrite: $('messages-write').value, chatsInvite: $('chats-invite').value, closed: $('profile-type').value === 'closed' };
    for (const [id, key] of Object.entries(reads)) privacy[key] = $(id).value;
    for (const [id, key] of Object.entries(shows)) privacy[key] = $(id).checked;
    await api.updatePrivacy(privacy);
  });
}

// initInterface is "Интерфейс", "Часовой пояс" and "Левое меню" of
// openvk: saved, they show at once, in the menu on the left too.
function initInterface(me) {
  const ui = me.interface;
  const device = deviceTheme();

  fill($('avatar-style'), avatarStyles, ui.avatarStyle);
  fill($('theme-select'), themes, device ?? ui.theme);
  fill($('rating'), ratings, ui.hideRating ? 'hide' : 'show');
  fill($('nsfw'), nsfwContents, ui.nsfw);
  fill($('post-view'), postViews, ui.postView);
  fill($('main-page'), mainPages, ui.mainPage);
  fill($('timezone'), timezones, ui.timezone === null ? '' : String(ui.timezone));
  $('theme-device').checked = device !== null;
  $('menu-items').replaceChildren(...menuItems.map(([item, text]) => el('label', {},
    el('input', { type: 'checkbox', value: item, checked: !ui.hiddenMenu.includes(item) }), text)));

  onSubmit($('interface'), async () => {
    const hiddenMenu = [...$('menu-items').querySelectorAll('input')].filter((box) => !box.checked).map((box) => box.value);
    // A style for this device alone stays in the browser: the settings
    // keep the one they had.
    const onDevice = $('theme-device').checked;

    const updated = await api.updateInterface({
      avatarStyle: $('avatar-style').value,
      theme: onDevice ? me.interface.theme : $('theme-select').value,
      hideRating: $('rating').value === 'hide',
      nsfw: $('nsfw').value,
      postView: $('post-view').value,
      mainPage: $('main-page').value,
      timezone: $('timezone').value === '' ? null : Number($('timezone').value),
      hiddenMenu,
    });

    setDeviceTheme(onDevice ? $('theme-select').value : null);
    Object.assign(me, updated);
    applyInterface(me);
  });
}

// sessionRow is one place the person is signed in at: what from, the
// address, when; "Завершить" signs that place out. The session this page
// is open in is ended by "выход" in the header.
function sessionRow(session, onEnd) {
  const facts = [
    session.ip && `IP-адрес: ${session.ip}`,
    `вход: ${formatDateTime(session.createdAt)}`,
    `последняя активность: ${formatDateTime(session.lastUsedAt)}`,
  ].filter(Boolean);

  return el('div', { className: 'session-row' },
    el('div', { className: 'session-device' },
      el('b', { textContent: deviceName(session.userAgent), title: session.userAgent }),
      session.current && el('span', { className: 'hint', textContent: ' — текущая сессия' }),
      el('div', { className: 'hint', textContent: facts.join(' | ') }),
    ),
    !session.current && el('a', { textContent: 'Завершить', onclick: () => onEnd(session) }),
  );
}

// initSecurity: openvk's "Безопасность" has "Сессии" with "Сбросить все
// сессии"; ours also lists them, each with "Завершить".
function initSecurity() {
  // run does what a link or the button asks and draws the list anew.
  async function run(action) {
    hideError(error);
    $('notice').hidden = true;

    try {
      await action();
      await show();
    } catch (e) {
      showError(error, e);
    }
  }

  async function show() {
    const { sessions } = await api.sessions();

    $('sessions-count').textContent = sessionsCount(sessions.length);
    $('sessions').replaceChildren(...sessions.map((s) => sessionRow(s, (ended) => run(() => api.endSession(ended.id)))));
    $('end-sessions-block').hidden = sessions.every((s) => s.current);
  }

  $('end-sessions').addEventListener('click', () => run(async () => {
    await api.endOtherSessions();
    $('notice').replaceChildren(el('b', { textContent: 'Все остальные сессии сброшены' }));
    $('notice').hidden = false;
  }));

  show().catch((e) => showError(error, e));
}

// say shows the notice above the tab: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  $('notice').hidden = false;
  $('notice').scrollIntoView({ block: 'nearest' });
}

function showVotes(me) {
  $('votes').textContent = me.votes ?? 0;
  $('votes-word').textContent = plural(me.votes ?? 0, ['голос', 'голоса', 'голосов']);
}

// openTransfer is openvk's "Передача голосов": how many and to whom, by
// the address of their page.
function openTransfer(me) {
  const modal = openModal('Передача голосов', { tag: 'form' });
  const votes = el('input', { type: 'text', inputMode: 'numeric', pattern: '[0-9]{1,7}', maxLength: 7, required: true, autocomplete: 'off' });
  const address = el('input', { type: 'text', maxLength: 64, required: true, spellcheck: false, autocomplete: 'off' });
  const failed = el('div', { className: 'error', hidden: true });
  const send = button('Передать голоса', null, { type: 'submit' });

  modal.box.classList.add('votes-transfer');
  modal.body.append(
    el('p', { textContent: 'Вы можете отправить в подарок или передать часть голосов другому человеку.' }),
    el('p', {}, 'Ваш текущий баланс: ', el('b', { textContent: votesCount(me.votes ?? 0) })),
    el('label', {}, 'Количество голосов', votes),
    el('label', {}, 'Адрес получателя', address),
    failed,
  );
  modal.actions.append(send, button('Отмена', modal.close, { secondary: true }));

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(failed);
    send.disabled = true;

    try {
      // The address is as it stands in the address bar: "ann", "@ann" or
      // the whole link.
      const nickname = address.value.trim().replace(/^.*\//, '').replace(/^@/, '');
      const recipient = await api.profile(nickname);
      const sent = Number(votes.value);
      const left = await api.transferVotes(recipient.nickname, sent);

      me.votes = left.votes;
      showVotes(me);
      modal.close();
      hideError(error);
      say('Голоса переданы', 'Вы успешно передали ', el('b', { textContent: votesCount(sent) }), ' — ',
        el('a', { href: pagePath(recipient), textContent: fullName(recipient) }), '.');
    } catch (e) {
      showError(failed, e.status === 404 ? 'Получатель не найден.' : e);
      send.disabled = false;
    }
  });
  votes.focus();
}

// initFinance is openvk's "Голоса": what is on the account, "Есть
// ваучер?" and "передать голоса".
function initFinance(me) {
  showVotes(me);
  $('transfer-link').addEventListener('click', () => openTransfer(me));
}

// initTopUp is "Есть ваучер?": the serial number in four fields of six,
// as on openvk — a full field passes on to the next, Backspace in an
// empty one goes back, and a pasted number fills them all.
function initTopUp(me) {
  const keys = [...document.querySelectorAll('.voucher-key')];
  const clean = (text) => text.replace(/[^a-z0-9]/gi, '').toUpperCase();

  function spread(text, from) {
    let rest = clean(text);

    for (let i = from; i < keys.length && rest; i++) {
      keys[i].value = rest.slice(0, 6);
      rest = rest.slice(6);
      keys[Math.min(i + (keys[i].value.length === 6 ? 1 : 0), keys.length - 1)].focus();
    }
  }

  keys.forEach((key, i) => {
    key.addEventListener('input', () => {
      key.value = clean(key.value);
      if (key.value.length === 6 && keys[i + 1]) keys[i + 1].focus();
    });
    key.addEventListener('keydown', (event) => {
      if (event.key === 'Backspace' && !key.value && keys[i - 1]) keys[i - 1].focus();
    });
    key.addEventListener('paste', (event) => {
      event.preventDefault();
      spread(event.clipboardData.getData('text'), i);
    });
  });

  $('voucher-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const submit = $('voucher-form').querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const got = await api.redeemVoucher(keys.map((key) => key.value).join(''));

      me.votes = got.balance;
      for (const key of keys) key.value = '';
      say('Ваучер активирован', `Вам начислено: ${voucherGave(got)}. Этим кодом вы больше не сможете его активировать. `,
        el('a', { href: '/settings?act=finance', textContent: `На вашем счету ${votesCount(got.balance)}.` }));
    } catch (e) {
      showError(error, e);
    } finally {
      submit.disabled = false;
    }
  });

  keys[0].focus();
}

// initVouchers is the administrators' "Ваучеры": every voucher with what
// it gives and how it is used, and the form that makes one.
function initVouchers() {
  let cursor = '';

  function row(voucher) {
    const tr = el('tr', {},
      el('td', { className: 'voucher-code', textContent: voucherCode(voucher.code) }),
      el('td', { textContent: voucher.votes }),
      el('td', { textContent: voucher.rating }),
      el('td', { textContent: voucherUsages(voucher.usagesLeft) }),
      el('td', { textContent: voucher.activations }),
      el('td', { textContent: voucherStatus(voucher) }),
      el('td', {}, el('a', { textContent: 'Удалить', onclick: () => remove(voucher, tr) })),
    );

    return tr;
  }

  function showEmpty() {
    const none = !$('voucher-rows').children.length;
    $('voucher-table').hidden = none;
    $('no-vouchers').hidden = !none;
  }

  async function remove(voucher, tr) {
    hideError(error);
    $('notice').hidden = true;

    try {
      await api.deleteVoucher(voucher.id);
      tr.remove();
      showEmpty();
    } catch (e) {
      showError(error, e);
    }
  }

  async function more() {
    hideError(error);

    try {
      const page = await api.vouchers(cursor);

      $('voucher-rows').append(...page.vouchers.map(row));
      cursor = page.nextCursor;
      $('more-vouchers').hidden = !cursor;
      showEmpty();
    } catch (e) {
      showError(error, e);
    }
  }

  $('more-vouchers').addEventListener('click', more);

  $('voucher-create').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const submit = $('voucher-create').querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const voucher = await api.createVoucher({
        code: $('voucher-code').value.trim(),
        votes: Number($('voucher-votes').value),
        rating: Number($('voucher-rating').value),
        usages: Number($('voucher-usages').value),
      });

      $('voucher-rows').prepend(row(voucher));
      showEmpty();
      $('voucher-code').value = '';
      say('Ваучер создан', 'Серийный номер: ', el('b', { className: 'voucher-code', textContent: voucherCode(voucher.code) }));
    } catch (e) {
      showError(error, e);
    } finally {
      submit.disabled = false;
    }
  });

  more();
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else if (!me.profileCompleted) {
  // Without a page there is nothing to set up yet: fill it in first.
  location.replace('/edit');
} else {
  // "Ваучеры" is for those who make them; ?act=finance.top-up is a page
  // of the tab "Голоса".
  const known = Object.hasOwn(tabs, asked) && (asked !== 'vouchers' || me.admin);
  const act = known ? asked : 'main';

  renderChrome(me);
  $('tab-vouchers').hidden = !me.admin;
  $('tab-' + act.split('.')[0]).classList.add('active');
  $(act.replaceAll('.', '-')).hidden = false;
  (tabs[act] ?? initMain)(me);
  $('loading').hidden = true;
}
