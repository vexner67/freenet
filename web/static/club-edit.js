// "Редактировать группу", as on openvk: the name, the description, the
// address of the page, the site, who may write on the wall and what the
// page shows of its admins ("Список админов"); an event ("встреча") also
// has when it starts and ends. Only the admins get here; the club service
// checks it again.

import { api, currentUser } from './api.js';
import { adminsDisplays, inputDate, inputTime, localTime } from './labels.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/edit'.length));

function fill(club) {
  const path = '/club/' + encodeURIComponent(clubRef(club));

  const heading = club.kind === 'event' ? 'Редактировать встречу' : 'Редактировать группу';

  document.title = `${heading} | Freenet`;
  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ` » ${heading}`);
  $('tab-main').href = path + '/edit';
  $('tab-backdrop').href = path + '/backdrop';
  $('tab-members').href = path + '/members';
  $('name').value = club.name;
  $('description').value = club.description;
  $('address').value = club.address;
  $('website').value = club.website;
  $('wall').value = club.wall;
  $('admins-display').replaceChildren(...adminsDisplays.map(([value, text]) => el('label', {},
    el('input', { type: 'radio', name: 'admins-display', value, checked: value === club.adminsDisplay }), text)));
  $('open-topics').checked = club.openTopics;
  $('topics-above-wall').checked = club.topicsAboveWall;
  fillSchedule(club);
  preview();
}

// fillSchedule shows "Начало" and "Окончание" of an event.
function fillSchedule(club) {
  const event = club.kind === 'event';

  $('start-row').hidden = !event;
  $('end-row').hidden = !event;
  if (!event) return;

  $('start-date').value = inputDate(club.startsAt);
  $('start-time').value = inputTime(club.startsAt);
  $('has-end').checked = Boolean(club.endsAt);
  $('end').hidden = !club.endsAt;
  $('end-date').value = club.endsAt ? inputDate(club.endsAt) : '';
  $('end-time').value = club.endsAt ? inputTime(club.endsAt) : '';
}

// schedule is what the form says of when the event is; nothing of a group.
function schedule(club) {
  if (club.kind !== 'event') return {};

  return {
    startsAt: localTime($('start-date').value, $('start-time').value),
    endsAt: $('has-end').checked ? localTime($('end-date').value, $('end-time').value) : '',
  };
}

// preview shows where the page will be: its address, or its ID without one.
function preview() {
  $('url-preview').textContent = `${location.host}/club/${$('address').value || 'ID'}`;
}

async function submit(event, club) {
  event.preventDefault();
  hideError($('error'));
  $('saved').hidden = true;
  $('submit').disabled = true;

  try {
    const saved = await api.updateClub(clubRef(club), {
      name: $('name').value,
      description: $('description').value,
      address: $('address').value.trim(),
      website: $('website').value.trim(),
      wall: $('wall').value,
      adminsDisplay: document.querySelector('input[name="admins-display"]:checked')?.value ?? 'creator',
      openTopics: $('open-topics').checked,
      topicsAboveWall: $('topics-above-wall').checked,
      ...schedule(club),
    });

    Object.assign(club, saved);
    fill(club);
    history.replaceState(null, '', '/club/' + encodeURIComponent(clubRef(club)) + '/edit');
    $('saved').hidden = false;
  } catch (e) {
    showError($('error'), e);
  } finally {
    $('submit').disabled = false;
  }
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const club = await api.club(ref);

    if (club.role !== 'admin' && club.role !== 'owner') {
      location.replace('/club/' + encodeURIComponent(clubRef(club)));
    } else {
      fill(club);
      $('address').addEventListener('input', preview);
      $('has-end').addEventListener('change', () => { $('end').hidden = !$('has-end').checked; });
      $('form').addEventListener('submit', (event) => submit(event, club));
      $('tabs').hidden = false;
      $('form').hidden = false;
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
    showError($('error'), e);
    $('error').hidden = false;
    $('form').hidden = false;
  }
}
