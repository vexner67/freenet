import { api, currentUser } from './api.js';
import { genders, relationships, worldviews, politicalViews, months } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';

const error = $('error');

function fill(select, options, value) {
  select.replaceChildren(...options.map(([v, text]) => el('option', { value: v, textContent: text })));
  select.value = value ?? '';
}

function fillDate(birthday) {
  const [year, month, day] = birthday ? birthday.split('-').map(Number) : [];
  const thisYear = new Date().getFullYear();

  fill($('birth-day'), [['', 'день'], ...Array.from({ length: 31 }, (_, i) => [String(i + 1), String(i + 1)])], day ? String(day) : '');
  fill($('birth-month'), [['', 'месяц'], ...months.map((m, i) => [String(i + 1), m])], month ? String(month) : '');
  fill($('birth-year'), [['', 'год'], ...Array.from({ length: 120 }, (_, i) => [String(thisYear - i), String(thisYear - i)])], year ? String(year) : '');
}

// birthday returns "YYYY-MM-DD", null for an empty date, or throws a
// message for a half-filled or impossible one (e.g. 31 February).
function birthday() {
  const parts = [$('birth-year').value, $('birth-month').value, $('birth-day').value];
  if (parts.every((p) => p === '')) return null;
  if (parts.some((p) => p === '')) throw 'Укажите дату рождения полностью или оставьте её пустой.';

  const [year, month, day] = parts.map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCMonth() !== month - 1) throw 'Такой даты не существует.';

  return date.toISOString().slice(0, 10);
}

function updateURLPreview() {
  $('url-preview').textContent = `${location.host}/${$('nickname').value.trim() || '…'}`;
}

function updateRelationshipLabels() {
  const select = $('relationship');
  fill(select, relationships($('gender').value), select.value);
}

function render(me) {
  $('name').value = me.name;
  $('surname').value = me.surname;
  $('nickname').value = me.nickname;
  $('status').value = me.status;

  fill($('gender'), genders, me.gender);
  fill($('relationship'), relationships(me.gender), me.relationship);
  fill($('worldview'), worldviews, me.worldview);
  fill($('political-views'), politicalViews, me.politicalViews);
  fillDate(me.birthday);

  $('show-birthday').checked = me.privacy.showBirthday;
  $('show-relationship').checked = me.privacy.showRelationship;
  $('show-worldview').checked = me.privacy.showWorldview;
  $('show-political-views').checked = me.privacy.showPoliticalViews;

  $('cancel').href = me.profileCompleted ? '/' + me.nickname : '/';
  $('welcome').hidden = me.profileCompleted;

  updateURLPreview();
}

async function save(event) {
  event.preventDefault();
  hideError(error);

  let profile;
  try {
    profile = {
      name: $('name').value.trim(),
      surname: $('surname').value.trim(),
      nickname: $('nickname').value.trim(),
      status: $('status').value.trim(),
      gender: $('gender').value,
      relationship: $('relationship').value,
      worldview: $('worldview').value,
      politicalViews: $('political-views').value,
      birthday: birthday(),
    };
  } catch (message) {
    showError(error, message);
    return;
  }

  const button = $('save');
  button.disabled = true;

  try {
    const me = await api.updateProfile(profile);
    await api.updatePrivacy({
      showBirthday: $('show-birthday').checked,
      showRelationship: $('show-relationship').checked,
      showWorldview: $('show-worldview').checked,
      showPoliticalViews: $('show-political-views').checked,
    });
    location.href = '/' + me.nickname;
  } catch (e) {
    showError(error, e);
    error.scrollIntoView({ block: 'nearest' });
  } finally {
    button.disabled = false;
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  render(me);

  $('nickname').addEventListener('input', updateURLPreview);
  $('gender').addEventListener('change', updateRelationshipLabels);
  $('form').addEventListener('submit', save);

  $('loading').hidden = true;
  $('form').hidden = false;
  (me.profileCompleted ? $('status') : $('name')).focus();
}
