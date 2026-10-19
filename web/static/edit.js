// openvk's "Редактировать страницу": the tabs "Основное", "Контакты",
// "Интересы", "Дополнительно", "Аватар" and "Фон", ?act= as on openvk;
// who sees them is in "Мои Настройки".

import { api, currentUser } from './api.js';
import { genders, relationships, takesPartner, worldviews, politicalViews, months } from './labels.js';
import { $, avatarURL, el, renderChrome, showError, hideError } from './layout.js';
import { openAvatarDialog } from './avatar.js';
import { initBackdropForm } from './backdrop.js';
import { tr } from './i18n.js';

const MAX_FIELDS = 7;

const error = $('error');
const tabs = {
  main: 'main', contacts: 'contacts', interests: 'interests', additional: 'additional', avatar: 'avatar-tab', backdrop: 'backdrop-tab',
};
const act = new URLSearchParams(location.search).get('act') ?? 'main';
const current = act in tabs ? act : 'main';

function fill(select, options, value) {
  select.replaceChildren(...options.map(([v, text]) => el('option', { value: v, textContent: text })));
  select.value = value ?? '';
}

function fillDate(birthday) {
  const [year, month, day] = birthday ? birthday.split('-').map(Number) : [];
  const thisYear = new Date().getFullYear();

  fill($('birth-day'), [['', tr('день')], ...Array.from({ length: 31 }, (_, i) => [String(i + 1), String(i + 1)])], day ? String(day) : '');
  fill($('birth-month'), [['', tr('месяц')], ...months.map((m, i) => [String(i + 1), m])], month ? String(month) : '');
  fill($('birth-year'), [['', tr('год')], ...Array.from({ length: 120 }, (_, i) => [String(thisYear - i), String(thisYear - i)])], year ? String(year) : '');
}

// birthday returns "YYYY-MM-DD", null for an empty date, or throws a
// message for a half-filled or impossible one (e.g. 31 February).
function birthday() {
  const parts = [$('birth-year').value, $('birth-month').value, $('birth-day').value];
  if (parts.every((p) => p === '')) return null;
  if (parts.some((p) => p === '')) throw tr('Укажите дату рождения полностью или оставьте её пустой.');

  const [year, month, day] = parts.map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCMonth() !== month - 1) throw tr('Такой даты не существует.');

  return date.toISOString().slice(0, 10);
}

function updateURLPreview() {
  $('url-preview').textContent = `${location.host}/${$('nickname').value.trim() || '…'}`;
}

// updateRelationship agrees the labels with the pronouns and asks whom
// the relationship names, when it names someone.
function updateRelationship() {
  const select = $('relationship');
  fill(select, relationships($('gender').value), select.value);
  $('partner').hidden = !takesPartner(select.value);
}

// saved shows openvk's "Изменения сохранены" over the form.
function saved() {
  $('notice').textContent = tr('Изменения сохранены. Новые данные появятся на вашей странице.');
  $('notice').hidden = false;
  $('notice').scrollIntoView({ block: 'nearest' });
}

// submit runs a save of a tab with its button disabled; errors show above.
function onSubmit(form, save) {
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;

    try {
      await save();
    } catch (e) {
      showError(error, e);
      error.scrollIntoView({ block: 'nearest' });
    } finally {
      button.disabled = false;
    }
  });
}

function initMain(me) {
  $('name').value = me.name;
  $('surname').value = me.surname;
  $('nickname').value = me.nickname;
  $('status').value = me.status;
  $('hometown').value = me.hometown;
  $('partner').value = me.partner?.nickname ?? '';

  fill($('gender'), genders, me.gender);
  fill($('relationship'), relationships(me.gender), me.relationship);
  fill($('worldview'), worldviews, me.worldview);
  fill($('political-views'), politicalViews, me.politicalViews);
  fillDate(me.birthday);
  $('birth-year-shown').value = me.hideBirthYear ? 'hide' : '';
  updateRelationship();

  $('cancel').href = me.profileCompleted ? '/' + me.nickname : '/';
  $('welcome').hidden = me.profileCompleted;
  updateURLPreview();

  $('nickname').addEventListener('input', updateURLPreview);
  $('gender').addEventListener('change', updateRelationship);
  $('relationship').addEventListener('change', updateRelationship);

  onSubmit($('main'), async () => {
    const relationship = $('relationship').value;
    const profile = {
      name: $('name').value.trim(),
      surname: $('surname').value.trim(),
      nickname: $('nickname').value.trim(),
      status: $('status').value.trim(),
      hometown: $('hometown').value.trim(),
      gender: $('gender').value,
      relationship,
      partner: takesPartner(relationship) ? $('partner').value.trim() : '',
      worldview: $('worldview').value,
      politicalViews: $('political-views').value,
      birthday: birthday(),
      hideBirthYear: $('birth-year-shown').value === 'hide',
    };

    const updated = await api.updateProfile(profile);
    // The first fill of the page leads to it; later ones stay, as the
    // other tabs do, with "Изменения сохранены".
    if (!me.profileCompleted) {
      location.href = '/' + updated.nickname;
      return;
    }
    $('cancel').href = '/' + updated.nickname;
    saved();
  });

  (me.profileCompleted ? $('status') : $('name')).focus();
}

const contactInputs = { email: 'contact-email', telegram: 'telegram', website: 'website', city: 'city', address: 'address' };

function initContacts(me) {
  for (const [key, id] of Object.entries(contactInputs)) $(id).value = me.contacts[key];

  onSubmit($('contacts'), async () => {
    const contacts = Object.fromEntries(Object.entries(contactInputs).map(([key, id]) => [key, $(id).value.trim()]));
    const updated = await api.updateContacts(contacts);
    // The forms are normalized: "@name" becomes "name", a scheme is added.
    for (const [key, id] of Object.entries(contactInputs)) $(id).value = updated.contacts[key];
    saved();
  });
}

const interestInputs = {
  interests: 'interests-text', music: 'music', films: 'films', shows: 'shows',
  books: 'books', quotes: 'quotes', games: 'games', about: 'about',
};

function initInterests(me) {
  for (const [key, id] of Object.entries(interestInputs)) $(id).value = me.interests[key];

  onSubmit($('interests'), async () => {
    await api.updateInterests(Object.fromEntries(Object.entries(interestInputs).map(([key, id]) => [key, $(id).value])));
    saved();
  });
}

// fieldNode is one field of "Дополнительно", as openvk's form shows it:
// "Название", "Текст" and "Отображение", with a cross to take it away.
function fieldNode(field = { name: '', text: '', place: 'contacts' }) {
  const name = el('input', { type: 'text', maxLength: 50, value: field.name, className: 'field-name' });
  const text = el('textarea', { maxLength: 1000, value: field.text, className: 'field-text' });
  const place = el('select', { className: 'field-place' },
    el('option', { value: 'contacts', textContent: tr('в «контактах»') }),
    el('option', { value: 'interests', textContent: tr('в «интересах»') }));
  place.value = field.place;

  const node = el('table', { className: 'form extra-field' },
    el('tr', {}, el('th', { textContent: tr('Название') }), el('td', { className: 'control' }, name),
      el('td', {}, el('a', { className: 'field-remove', title: tr('Удалить поле'), textContent: '×', onclick: () => {
        node.remove();
        updateAddButton();
      } }))),
    el('tr', {}, el('th', { textContent: tr('Текст') }), el('td', { className: 'control' }, text)),
    el('tr', {}, el('th', { textContent: tr('Отображение') }), el('td', { className: 'control' }, place)),
  );

  return node;
}

function updateAddButton() {
  $('field-add').hidden = $('fields').children.length >= MAX_FIELDS;
}

function initAdditional(me) {
  $('fields').replaceChildren(...me.fields.map(fieldNode));
  updateAddButton();

  $('field-add').addEventListener('click', () => {
    $('fields').append(fieldNode());
    updateAddButton();
  });

  // Blank fields are left out, as openvk does.
  onSubmit($('additional'), async () => {
    const fields = [...$('fields').children]
      .map((node) => ({
        name: node.querySelector('.field-name').value.trim(),
        text: node.querySelector('.field-text').value.trim(),
        place: node.querySelector('.field-place').value,
      }))
      .filter((f) => f.name && f.text);

    const updated = await api.updateExtraFields(fields);
    $('fields').replaceChildren(...updated.fields.map(fieldNode));
    updateAddButton();
    saved();
  });
}

function initAvatar(me) {
  const show = (u) => { $('avatar').src = avatarURL(u, 'large'); };
  show(me);

  $('avatar-upload').addEventListener('click', () => openAvatarDialog({
    upload: async (blob) => {
      hideError(error);
      show(await api.uploadAvatar(blob));
      saved();
    },
  }));
}

// initBackdrop is openvk's "Фон страницы" of the user (backdrop.js).
function initBackdrop(me) {
  initBackdropForm(me.backdrop, {
    upload: async (side, file) => (await api.setBackdrop(side, file)).backdrop,
    remove: async () => (await api.deleteBackdrop()).backdrop,
  });
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  // Until the profile is filled in, only "Основное" makes sense.
  const tab = me.profileCompleted ? current : 'main';
  $('tab-' + tab).classList.add('active');
  $(tabs[tab]).hidden = false;

  ({ main: initMain, contacts: initContacts, interests: initInterests, additional: initAdditional, avatar: initAvatar, backdrop: initBackdrop })[tab](me);

  $('loading').hidden = true;
}
