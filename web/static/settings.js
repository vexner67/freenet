// openvk's "Мои Настройки" at /settings: "Основное" (the e-mail, the
// page address and "удалить свою страницу"), "Безопасность" (?act=security:
// where you are signed in, and the key messages are sealed with) and "Приватность" (?act=privacy:
// who sees the page and its parts and who writes to you) and "Внешний вид"
// (?act=interface: the style, the photos and the posts, the clock, where
// the site opens and what the left menu shows); "Чёрный список" is a page of its own.
// "Голоса" (?act=finance: the balance and "передать голоса";
// ?act=finance.top-up: "Есть ваучер?"). The vouchers themselves are made in
// "Админ-панель" (admin.js); ?act=vouchers, where they were, leads there.

import { api, currentUser } from './api.js';
import { deviceName, formatDateTime, keyStateText, sessionsCount, plural, rubles, voteRate, votesCount, voucherGave, audiences, avatarStyles, mainPages, menuItems, nsfwContents, pageReads, postViews, onlineReads, ratings, themes, timezones, wallWrites, messagesWrites, chatsInvites, profileTypes, pushStateTexts } from './labels.js';
import { $, applyInterface, deviceTheme, el, fullName, pagePath, setDeviceTheme, profilePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { openDeactivate } from './deactivate.js';
import { disablePush, enablePush, pushState } from './push.js';
import { ensureKey, exportKeys, forgetKeys, hasKeys, importKeys, KEY_READY, newCode, newKey } from './e2ee.js';

const error = $('error');
const tabs = {
  security: initSecurity, privacy: initPrivacy, interface: initInterface,
  finance: initFinance, 'finance.top-up': initTopUp,
};
const asked = new URLSearchParams(location.search).get('act');
// What a vote is worth, asked with the page for "Голоса"; 0 when unknown.
const rate = asked === 'finance' ? api.voteRate().then((r) => r.kopecks, () => 0) : Promise.resolve(0);

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
  showPush(me).catch((e) => showError(error, e));
}

// showPush is "Уведомления на этом устройстве": whether this browser gets
// the pushes, and the button that changes it.
async function showPush(me) {
  const state = await pushState();
  $('push-state').textContent = pushStateTexts[state];

  const change = async (turn) => {
    hideError(error);
    try {
      await turn();
    } catch (e) {
      showError(error, e);
    }
    await showPush(me);
  };

  const actions = {
    off: () => button('Включить уведомления', () => change(() => enablePush(me))),
    on: () => button('Выключить уведомления', () => change(disablePush)),
  };
  $('push-actions').replaceChildren(...(actions[state] ? [actions[state]()] : []));
}

const reads = {
  'page-read': 'pageRead', 'info-read': 'infoRead', 'online-read': 'onlineRead',
  'clubs-read': 'clubsRead', 'photos-read': 'photosRead', 'friends-read': 'friendsRead',
  'gifts-read': 'giftsRead',
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
  // As openvk's: the box where the label would be, the name beside it.
  $('menu-items').replaceChildren(...menuItems.map(([item, text]) => el('tr', {},
    el('th', { className: 'check-cell' },
      el('input', { type: 'checkbox', id: 'menu-' + item, value: item, checked: !ui.hiddenMenu.includes(item) })),
    el('td', {}, el('label', { htmlFor: 'menu-' + item, textContent: text })))));

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
function initSecurity(me) {
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
  showKey(me);
}

// showKey is what this browser has of the key messages of dialogs are
// sealed with (e2ee.js), and what can be done with it.
async function showKey(me, state) {
  try {
    state ??= await ensureKey(me.id, { force: true });
  } catch (e) {
    showError(error, e);
    return;
  }

  const ready = state === KEY_READY;
  const again = (next) => showKey(me, next);

  $('key-state').textContent = ready ? 'На этом устройстве есть ключ шифрования сообщений.' : keyStateText(state);
  $('key-actions').replaceChildren(...[
    ready && button('Сохранить ключ в файл', () => openExport(me)),
    button('Загрузить ключ из файла', () => openImport(me, again), { secondary: ready }),
    button('Создать новый ключ', () => openNewKey(me, again), { secondary: true }),
    hasKeys(me.id) && button('Удалить ключ с этого устройства', () => openForget(me, again), { secondary: true }),
  ].filter(Boolean));
}

// openExport saves the keys into a file locked with a code, which the
// window shows: both are needed on the other device.
async function openExport(me) {
  const code = newCode();
  const file = new Blob([await exportKeys(me.id, code)], { type: 'application/json' });
  const link = el('a', { href: URL.createObjectURL(file), download: `freenet-key-${me.nickname}.json`, textContent: 'Скачать файл ещё раз' });
  const modal = openModal('Перенос ключа');

  modal.body.append(
    el('p', {}, 'Файл с ключом сохраняется. Перенесите его на другое устройство и загрузите там в «Настройках» → «Безопасность», введя этот код:'),
    el('p', { className: 'safety-code', textContent: code }),
    el('p', { className: 'hint', textContent: 'Код показывается один раз: запишите его. С файлом и кодом можно прочитать ваши сообщения — не храните их вместе и никому не передавайте.' }),
    link,
  );
  modal.actions.append(button('Закрыть', modal.close));
  link.click();
}

// openImport takes keys from a file openExport made, with its code.
function openImport(me, done) {
  const modal = openModal('Загрузка ключа', { tag: 'form' });
  const fail = el('div', { className: 'error', hidden: true });
  const file = el('input', { type: 'file', accept: 'application/json,.json', ariaLabel: 'Файл с ключом' });
  const code = el('input', { type: 'text', maxLength: 40, autocomplete: 'off', placeholder: 'XXXX-XXXX-XXXX-XXXX-XXXX', ariaLabel: 'Код' });
  const load = button('Загрузить', null, { type: 'submit' });

  modal.body.append(fail, el('p', {}, 'Файл с ключом:'), file, el('p', {}, 'Код, который показало другое устройство:'), code);
  modal.actions.append(load, button('Отмена', modal.close, { secondary: true }));
  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(fail);
    load.disabled = true;

    try {
      if (!file.files[0]) throw 'Выберите файл с ключом.';

      const state = await importKeys(me.id, await file.files[0].text(), code.value);

      modal.close();
      say(state === KEY_READY ? 'Ключ загружен' : 'Ключ загружен, но он не тот, что действует сейчас: загрузите файл, сохранённый позже.');
      done(state);
    } catch (e) {
      showError(fail, e);
      load.disabled = false;
    }
  });
}

// openNewKey sets up a new key: what the old one sealed opens only where
// it is kept.
function openNewKey(me, done) {
  const modal = openModal('Новый ключ шифрования');
  const fail = el('div', { className: 'error', hidden: true });
  const make = button('Создать', async () => {
    hideError(fail);
    make.disabled = true;

    try {
      const state = await newKey(me.id);

      modal.close();
      say('Новый ключ создан', 'Собеседники увидят, что ваш ключ изменился. Перенесите его на другие свои устройства.');
      done(state);
    } catch (e) {
      showError(fail, e);
      make.disabled = false;
    }
  });

  modal.body.append(fail, el('p', {}, 'Новые сообщения будут шифроваться новым ключом. Сообщения, зашифрованные прежним, откроются только там, где он есть, а на других своих устройствах нужно будет загрузить новый ключ.'));
  modal.actions.append(make, button('Отмена', modal.close, { secondary: true }));
}

// openForget removes the keys from this browser, as on a computer that is
// not yours.
function openForget(me, done) {
  const modal = openModal('Удаление ключа');

  modal.body.append(el('p', {}, 'После удаления на этом устройстве нельзя будет читать и писать личные сообщения, пока вы не загрузите ключ снова. Если ключ не сохранён в файл или на другом устройстве, сообщения будет не прочитать нигде.'));
  modal.actions.append(button('Удалить', () => {
    forgetKeys(me.id);
    modal.close();
    done();
  }), button('Отмена', modal.close, { secondary: true }));
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
  $('votes-rubles').textContent = kopecksPerVote ? `≈ ${rubles((me.votes ?? 0) * kopecksPerVote)}` : '';
}

let kopecksPerVote = 0;

// showRate is what a vote is worth and what the balance is worth with it.
async function showRate(me) {
  kopecksPerVote = await rate;
  if (!kopecksPerVote) return;

  $('vote-rate').textContent = `Курс: ${voteRate(kopecksPerVote)}.`;
  $('vote-rate').hidden = false;
  showVotes(me);
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
  showRate(me);
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

const me = await currentUser().catch(() => null);
if (asked === 'vouchers') {
  location.replace(me?.admin ? '/admin/vouchers' : '/settings');
} else if (!me) {
  location.replace('/');
} else if (!me.profileCompleted) {
  // Without a page there is nothing to set up yet: fill it in first.
  location.replace('/edit');
} else {
  // ?act=finance.top-up is a page of the tab "Голоса".
  const act = Object.hasOwn(tabs, asked) ? asked : 'main';

  renderChrome(me);
  $('tab-' + act.split('.')[0]).classList.add('active');
  $(act.replaceAll('.', '-')).hidden = false;
  (tabs[act] ?? initMain)(me);
  $('loading').hidden = true;
}
