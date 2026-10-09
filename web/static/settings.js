// openvk's "Мои Настройки" at /settings: "Основное" (the e-mail, the
// page address and "удалить свою страницу") and "Приватность" (?act=privacy:
// who sees the page and its parts and who writes to you); "Чёрный список"
// is a page of its own.

import { api, currentUser } from './api.js';
import { audiences, pageReads, wallWrites, messagesWrites } from './labels.js';
import { $, el, profilePath, renderChrome, showError, hideError } from './layout.js';
import { openDeactivate } from './deactivate.js';

const error = $('error');
const act = new URLSearchParams(location.search).get('act') === 'privacy' ? 'privacy' : 'main';

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

const reads = { 'page-read': 'pageRead', 'info-read': 'infoRead', 'clubs-read': 'clubsRead', 'photos-read': 'photosRead', 'friends-read': 'friendsRead' };
const shows = { 'show-birthday': 'showBirthday', 'show-relationship': 'showRelationship', 'show-worldview': 'showWorldview', 'show-political-views': 'showPoliticalViews' };

function initPrivacy(me) {
  for (const [id, key] of Object.entries(reads)) fill($(id), id === 'page-read' ? pageReads : audiences, me.privacy[key]);
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
  (act === 'privacy' ? initPrivacy : initMain)(me);
  $('loading').hidden = true;
}
