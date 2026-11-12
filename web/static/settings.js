// openvk's "Мои Настройки" at /settings: "Основное" (the e-mail, the
// page address and "удалить свою страницу"), "Безопасность" (?act=security:
// the second step of signing in, twofactor.js, where you are signed in,
// and the key messages are sealed with), "Приватность" (?act=privacy:
// who sees the page and its parts and who writes to you) and "Внешний вид"
// (?act=interface: "Интерфейс", "Часовой пояс" and "Левое меню", each
// saved by its own button, as openvk's); the blacklist has its own page
// linked among the tabs. ?act=vouchers leads to "Админ-панель".
// The tabs change without a new page; a saved form answers over the page,
// so that nothing under the reader moves.

import { api, currentUser } from './api.js';
import { deviceName, formatDateTime, keyStateText, sessionsCount, audiences, avatarStyles, mainPages, menuItems, nsfwContents, pageReads, postViews, onlineReads, ratings, themes, timezones, wallWrites, messagesWrites, chatsInvites, profileTypes, pushStateTexts } from './labels.js';
import { $, applyInterface, deviceTheme, el, fediverseAccountNode, setDeviceTheme, profilePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { openDeactivate } from './deactivate.js';
import { disablePush, enablePush, pushState } from './push.js';
import { ensureKey, exportKeys, forgetKeys, hasKeys, importKeys, KEY_READY, newCode, newKey } from './e2ee.js';
import { showTwoFactor } from './twofactor.js';
import { helpButton } from './game.js';
import { tr } from './i18n.js';

const error = $('error');
const tabs = { main: initMain, security: initSecurity, privacy: initPrivacy, interface: initInterface };
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
      saved();
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  });
}

// How long "Изменения сохранены" stays over the page.
const noticeTime = 3000;
let noticeTimer = 0;

function showNotice() {
  $('notice').hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => { $('notice').hidden = true; }, noticeTime);
}

// saved is what a saved form answers: openvk's "Изменения сохранены".
function saved() {
  $('notice').replaceChildren(el('b', { textContent: tr('Изменения сохранены') }), el('br'), tr('Новые данные появятся на вашей странице.'));
  showNotice();
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
    off: () => button(tr('Включить уведомления'), () => change(() => enablePush(me))),
    on: () => button(tr('Выключить уведомления'), () => change(disablePush)),
  };
  $('push-actions').replaceChildren(...(actions[state] ? [actions[state]()] : []));
}

const reads = {
  'page-read': 'pageRead', 'info-read': 'infoRead', 'online-read': 'onlineRead',
  'clubs-read': 'clubsRead', 'friends-read': 'friendsRead',
  'gifts-read': 'giftsRead', 'audios-read': 'audiosRead', 'listening-read': 'listeningRead',
};
const readOptions = { 'page-read': pageReads, 'online-read': onlineReads };
const shows = { 'show-birthday': 'showBirthday', 'show-relationship': 'showRelationship', 'show-worldview': 'showWorldview', 'show-political-views': 'showPoliticalViews' };

function initPrivacy(me) {
  for (const [id, key] of Object.entries(reads)) fill($(id), readOptions[id] ?? audiences, me.privacy[key]);
  for (const [id, key] of Object.entries(shows)) $(id).checked = me.privacy[key];
  fill($('wall-write'), wallWrites, me.privacy.wallWrite);
  fill($('messages-write'), messagesWrites, me.privacy.messagesWrite);
  $('messages-prime').checked = Boolean(me.privacy.messagesPrime);
  $('message-price').value = String(me.privacy.messagePrice ?? 0);
  fill($('chats-invite'), chatsInvites, me.privacy.chatsInvite);
  // An empty value is a page saved before the setting came: anyone.
  fill($('gifts-send'), chatsInvites, me.privacy.giftsSend || 'everyone');
  fill($('profile-type'), profileTypes, me.privacy.closed ? 'closed' : 'open');
  showFediverse(me);

  onSubmit($('privacy'), async () => {
    const privacy = { wallWrite: $('wall-write').value, messagesWrite: $('messages-write').value, chatsInvite: $('chats-invite').value, giftsSend: $('gifts-send').value, closed: $('profile-type').value === 'closed' };
    privacy.messagesPrime = $('messages-prime').checked;
    privacy.messagePrice = Math.max(0, Math.floor(Number($('message-price').value) || 0));
    for (const [id, key] of Object.entries(reads)) privacy[key] = $(id).value;
    for (const [id, key] of Object.entries(shows)) privacy[key] = $(id).checked;
    await api.updatePrivacy(privacy);
  });
}

// showFediverse is whether the page is open to the Fediverse, and the
// button that changes it; a site without federation has no such block.
async function showFediverse(me) {
  let status;
  try {
    status = await api.federation();
  } catch (e) {
    if (e.status !== 404) showError(error, e);
    return;
  }

  const address = `@${me.nickname}@${status.domain}`;
  $('fediverse-state').replaceChildren(...(status.open
    ? [tr('Ваша страница открыта для Fediverse. Её адрес там: '), el('b', { textContent: address }), '. ',
        tr('Подписчиков в других сетях: {0}.', status.followers), ' ',
        helpButton(tr('Чтобы подписаться, человеку из Mastodon достаточно ввести {0} в поиск. Закрыв страницу, вы удалите её из Fediverse: её подписчики там пропадут.', address))]
    : [tr('Ваша страница закрыта для Fediverse.')]));

  const change = async (turn) => {
    hideError(error);
    try {
      await turn();
    } catch (e) {
      showError(error, e);
    }
    await showFediverse(me);
  };

  $('fediverse-actions').replaceChildren(status.open
    ? button(tr('Закрыть страницу для Fediverse'), () => openCloseFediverse(() => change(api.closeFederation)))
    : button(tr('Открыть страницу для Fediverse'), () => change(api.openFederation)));
  $('fediverse').hidden = false;
  initFediverseReading();
}

let readingReady = false;

// initFediverseReading is "Чтение Fediverse": the people of other servers
// whose posts come to my news, and the field that adds one by address.
function initFediverseReading() {
  if (readingReady) return;
  readingReady = true;

  const field = $('fediverse-address');
  const subscribe = button(tr('Подписаться'), add);
  $('fediverse-subscribe').replaceChildren(subscribe);

  // The field is in the form of privacy: Enter adds, not saves it.
  field.addEventListener('keydown', (event) => {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    add();
  });

  async function add() {
    const address = field.value.trim();
    if (!address || subscribe.disabled) return;

    hideError(error);
    subscribe.disabled = true;
    try {
      await api.subscribeFediverse(address);
      field.value = '';
      await showSubscriptions();
    } catch (e) {
      showError(error, e);
    } finally {
      subscribe.disabled = false;
    }
  }

  showSubscriptions();
}

async function showSubscriptions() {
  let list;
  try {
    list = await api.fediverseSubscriptions();
  } catch (e) {
    showError(error, e);
    return;
  }

  $('fediverse-subscriptions').replaceChildren(...(list.accounts.length === 0
    ? [el('p', { className: 'hint center', textContent: tr('Вы ещё ни на кого не подписаны в Fediverse.') })]
    : list.accounts.map((account) => fediverseAccountNode(account, {
      action: tr('Отписаться'),
      run: async () => {
        hideError(error);
        await api.unsubscribeFediverse(account.id);
        await showSubscriptions();
      },
      onError: (e) => showError(error, e),
    }))));
}

function openCloseFediverse(done) {
  const modal = openModal(tr('Закрыть страницу для Fediverse'));

  modal.body.append(el('p', {}, tr('Серверы ваших подписчиков в Fediverse узнают, что страницы там больше нет, и подписчики пропадут. Открыть страницу можно снова, но подписываться на неё придётся заново.')));
  modal.actions.append(button(tr('Закрыть'), () => {
    modal.close();
    done();
  }), button(tr('Отмена'), modal.close, { secondary: true }));
}

// initInterface is "Интерфейс", "Часовой пояс" and "Левое меню" of
// openvk, three forms with a button each: saved, they show at once, in
// the menu on the left too. Each sends what it shows and what the others
// had when last saved, so a field changed but not saved in another stays
// as it was.
function initInterface(me) {
  const ui = me.interface;
  const device = deviceTheme();

  fill($('avatar-style'), avatarStyles, ui.avatarStyle);
  // The gold style is Prime's: offered with it, and kept by one
  // who chose it before Prime ended.
  const chosen = device ?? ui.theme;
  fill($('theme-select'), themes.filter(([v]) => v !== 'gold' || me.prime || chosen === 'gold'), chosen);
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

  // The endpoint replaces the whole interface. Build each request from
  // the last saved values, even when another form is submitted in flight.
  let pending = Promise.resolve();
  function save(changes, deviceStyle) {
    const saved = pending.then(async () => {
      const updated = await api.updateInterface({ ...me.interface, ...changes });
      me.interface = updated.interface;
      if (deviceStyle !== undefined) setDeviceTheme(deviceStyle);
      applyInterface(me);
    });
    pending = saved.catch(() => {});
    return saved;
  }

  onSubmit($('interface-look'), () => {
    // A style for this device alone stays in the browser: the settings
    // keep the one they had.
    const onDevice = $('theme-device').checked;
    const theme = $('theme-select').value;

    return save({
      avatarStyle: $('avatar-style').value,
      ...(!onDevice && { theme }),
      hideRating: $('rating').value === 'hide',
      nsfw: $('nsfw').value,
      postView: $('post-view').value,
      mainPage: $('main-page').value,
    }, onDevice ? theme : null);
  });

  onSubmit($('interface-timezone'), () => save({
    timezone: $('timezone').value === '' ? null : Number($('timezone').value),
  }));

  onSubmit($('interface-menu'), () => save({
    hiddenMenu: [...$('menu-items').querySelectorAll('input')].filter((box) => !box.checked).map((box) => box.value),
  }));
}

// sessionRow is one place the person is signed in at: what from, the
// address, when; "Завершить" signs that place out. The session this page
// is open in is ended by "выход" in the header.
function sessionRow(session, onEnd) {
  const facts = [
    session.ip && tr('IP-адрес: {0}', session.ip),
    tr('вход: {0}', formatDateTime(session.createdAt)),
    tr('последняя активность: {0}', formatDateTime(session.lastUsedAt)),
  ].filter(Boolean);

  return el('div', { className: 'session-row' },
    el('div', { className: 'session-device' },
      el('b', { textContent: deviceName(session.userAgent), title: session.userAgent }),
      session.current && el('span', { className: 'hint', textContent: tr(' — текущая сессия') }),
      el('div', { className: 'hint', textContent: facts.join(' | ') }),
    ),
    !session.current && el('a', { textContent: tr('Завершить'), onclick: () => onEnd(session) }),
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
    $('notice').replaceChildren(el('b', { textContent: tr('Все остальные сессии сброшены') }));
    $('notice').hidden = false;
  }));

  show().catch((e) => showError(error, e));
  showTwoFactor(error, say);
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

  $('key-state').textContent = ready ? tr('На этом устройстве есть ключ шифрования сообщений.') : keyStateText(state);
  $('key-actions').replaceChildren(...[
    ready && button(tr('Сохранить ключ в файл'), () => openExport(me)),
    button(tr('Загрузить ключ из файла'), () => openImport(me, again), { secondary: ready }),
    button(tr('Создать новый ключ'), () => openNewKey(me, again), { secondary: true }),
    hasKeys(me.id) && button(tr('Удалить ключ с этого устройства'), () => openForget(me, again), { secondary: true }),
  ].filter(Boolean));
}

// openExport saves the keys into a file locked with a code, which the
// window shows: both are needed on the other device.
async function openExport(me) {
  const code = newCode();
  const file = new Blob([await exportKeys(me.id, code)], { type: 'application/json' });
  const link = el('a', { href: URL.createObjectURL(file), download: `freenet-key-${me.nickname}.json`, textContent: tr('Скачать файл ещё раз') });
  const modal = openModal(tr('Перенос ключа'));

  modal.body.append(
    el('p', {}, tr('Файл с ключом сохраняется. Перенесите его на другое устройство и загрузите там в «Настройках» → «Безопасность», введя этот код:')),
    el('p', { className: 'safety-code', textContent: code }),
    el('p', { className: 'hint', textContent: tr('Код показывается один раз: запишите его. С файлом и кодом можно прочитать ваши сообщения — не храните их вместе и никому не передавайте.') }),
    link,
  );
  modal.actions.append(button(tr('Закрыть'), modal.close));
  link.click();
}

// openImport takes keys from a file openExport made, with its code.
function openImport(me, done) {
  const modal = openModal(tr('Загрузка ключа'), { tag: 'form' });
  const fail = el('div', { className: 'error', hidden: true });
  const file = el('input', { type: 'file', accept: 'application/json,.json', ariaLabel: tr('Файл с ключом') });
  const code = el('input', { type: 'text', maxLength: 40, autocomplete: 'off', placeholder: 'XXXX-XXXX-XXXX-XXXX-XXXX', ariaLabel: tr('Код') });
  const load = button(tr('Загрузить'), null, { type: 'submit' });

  modal.body.append(fail, el('p', {}, tr('Файл с ключом:')), file, el('p', {}, tr('Код, который показало другое устройство:')), code);
  modal.actions.append(load, button(tr('Отмена'), modal.close, { secondary: true }));
  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(fail);
    load.disabled = true;

    try {
      if (!file.files[0]) throw tr('Выберите файл с ключом.');

      const state = await importKeys(me.id, await file.files[0].text(), code.value);

      modal.close();
      say(state === KEY_READY ? tr('Ключ загружен') : tr('Ключ загружен, но он не тот, что действует сейчас: загрузите файл, сохранённый позже.'));
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
  const modal = openModal(tr('Новый ключ шифрования'));
  const fail = el('div', { className: 'error', hidden: true });
  const make = button(tr('Создать'), async () => {
    hideError(fail);
    make.disabled = true;

    try {
      const state = await newKey(me.id);

      modal.close();
      say(tr('Новый ключ создан'), tr('Собеседники увидят, что ваш ключ изменился. Перенесите его на другие свои устройства.'));
      done(state);
    } catch (e) {
      showError(fail, e);
      make.disabled = false;
    }
  });

  modal.body.append(fail, el('p', {}, tr('Новые сообщения будут шифроваться новым ключом. Сообщения, зашифрованные прежним, откроются только там, где он есть, а на других своих устройствах нужно будет загрузить новый ключ.')));
  modal.actions.append(make, button(tr('Отмена'), modal.close, { secondary: true }));
}

// openForget removes the keys from this browser, as on a computer that is
// not yours.
function openForget(me, done) {
  const modal = openModal(tr('Удаление ключа'));

  modal.body.append(el('p', {}, tr('После удаления на этом устройстве нельзя будет читать и писать личные сообщения, пока вы не загрузите ключ снова. Если ключ не сохранён в файл или на другом устройстве, сообщения будет не прочитать нигде.')));
  modal.actions.append(button(tr('Удалить'), () => {
    forgetKeys(me.id);
    modal.close();
    done();
  }), button(tr('Отмена'), modal.close, { secondary: true }));
}

// say shows the notice above the tab: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  showNotice();
}

const opened = new Set();

// openTab shows a tab in place; what it needs is asked for the first time
// only.
function openTab(act, heard = false) {
  for (const name of Object.keys(tabs)) {
    $(name).hidden = name !== act;
    $('tab-' + name).classList.toggle('active', name === act);
  }
  if (!opened.has(act)) {
    opened.add(act);
    tabs[act](me);
  }
  if (heard) {
    history.replaceState(null, '', act === 'main' ? '/settings' : `/settings?act=${act}`);
  }
}

function listenTabs() {
  for (const name of Object.keys(tabs)) {
    $('tab-' + name).addEventListener('click', (event) => {
      if (event.button || event.metaKey || event.ctrlKey || event.shiftKey) return;
      event.preventDefault();
      openTab(name, true);
    });
  }
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
  const act = Object.hasOwn(tabs, asked) ? asked : 'main';

  renderChrome(me);
  listenTabs();
  openTab(act);
  $('loading').hidden = true;
}
