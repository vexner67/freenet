// openvk's "Мои Настройки" at /settings: "Основное" (the e-mail, the
// page address and "удалить свою страницу") and "Приватность" (?act=privacy:
// who sees the page and its parts and who writes to you) and "Внешний вид"
// (?act=interface: the style, the photos and the posts, the clock, where
// the site opens and what the left menu shows); "Чёрный список" is a page of its own.

import { api, currentUser } from './api.js';
import { audiences, avatarStyles, mainPages, menuItems, nsfwContents, pageReads, postViews, onlineReads, ratings, themes, timezones, wallWrites, messagesWrites } from './labels.js';
import { $, applyInterface, deviceTheme, el, setDeviceTheme, profilePath, renderChrome, showError, hideError } from './layout.js';
import { openDeactivate } from './deactivate.js';

const error = $('error');
const tabs = { privacy: initPrivacy, interface: initInterface };
const asked = new URLSearchParams(location.search).get('act');
const act = Object.hasOwn(tabs, asked) ? asked : 'main';

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

// initMain: the page address is the nickname, a part of "Основное" of the
// page, so it goes back with the rest of it unchanged.
function initMain(me) {
  $('email').textContent = me.email;
  $('page-id').textContent = me.id;
  $('nickname').value = me.nickname;

  onSubmit($('address'), async () => {
    const updated = await api.updateProfile({
      name: me.name,
      surname: me.surname,
      nickname: $('nickname').value.trim(),
      status: me.status,
      hometown: me.hometown,
      gender: me.gender,
      relationship: me.relationship,
      partner: me.partner?.nickname ?? '',
      worldview: me.worldview,
      politicalViews: me.politicalViews,
      birthday: me.birthday || null,
      hideBirthYear: me.hideBirthYear,
    });
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
};
const readOptions = { 'page-read': pageReads, 'online-read': onlineReads };
const shows = { 'show-birthday': 'showBirthday', 'show-relationship': 'showRelationship', 'show-worldview': 'showWorldview', 'show-political-views': 'showPoliticalViews' };

function initPrivacy(me) {
  for (const [id, key] of Object.entries(reads)) fill($(id), readOptions[id] ?? audiences, me.privacy[key]);
  for (const [id, key] of Object.entries(shows)) $(id).checked = me.privacy[key];
  fill($('wall-write'), wallWrites, me.privacy.wallWrite);
  fill($('messages-write'), messagesWrites, me.privacy.messagesWrite);

  onSubmit($('privacy'), async () => {
    const privacy = { wallWrite: $('wall-write').value, messagesWrite: $('messages-write').value };
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

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else if (!me.profileCompleted) {
  // Without a page there is nothing to set up yet: fill it in first.
  location.replace('/edit');
} else {
  renderChrome(me);
  $('tab-' + act).classList.add('active');
  $(act).hidden = false;
  (tabs[act] ?? initMain)(me);
  $('loading').hidden = true;
}
