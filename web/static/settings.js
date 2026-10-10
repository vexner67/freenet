// openvk's "Мои Настройки" at /settings: "Основное" (the e-mail, the
// page address and "удалить свою страницу") and "Приватность" (?act=privacy:
// who sees the page and its parts and who writes to you) and "Внешний вид"
// (?act=interface: how the photos look, where the site opens and what the
// left menu shows); "Чёрный список" is a page of its own.

import { api, currentUser } from './api.js';
import { audiences, avatarStyles, mainPages, menuItems, pageReads, onlineReads, wallWrites, messagesWrites } from './labels.js';
import { $, applyInterface, el, profilePath, renderChrome, showError, hideError } from './layout.js';
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

// initInterface is "Интерфейс" and "Левое меню" of openvk: saved, they
// show at once, in the menu on the left too.
function initInterface(me) {
  fill($('avatar-style'), avatarStyles, me.interface.avatarStyle);
  fill($('main-page'), mainPages, me.interface.mainPage);
  $('menu-items').replaceChildren(...menuItems.map(([item, text]) => el('label', {},
    el('input', { type: 'checkbox', value: item, checked: !me.interface.hiddenMenu.includes(item) }), text)));

  onSubmit($('interface'), async () => {
    const hiddenMenu = [...$('menu-items').querySelectorAll('input')].filter((box) => !box.checked).map((box) => box.value);
    const updated = await api.updateInterface({ avatarStyle: $('avatar-style').value, mainPage: $('main-page').value, hiddenMenu });

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
