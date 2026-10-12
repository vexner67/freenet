// The members of a club ("Участники") and its admins ("Руководство"), as
// on openvk, the newest first. Its owner makes members admins and back,
// and hands the club over to an admin ("Назначить владельцем"); those who
// run it give each other a title ("Изменить комментарий") and hide each
// other from the rest ("Скрыт"). An event ("встреча") lists those who
// come ("Точно придут") and those who might ("Возможно придут") apart.

import { api, currentUser } from './api.js';
import { clubRoles, membersCount, requestsCount } from './labels.js';
import { $, clubRef, el, fullName, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { personRow } from './people.js';
import { separated } from './posts.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/members'.length));
const params = new URLSearchParams(location.search);
const admins = params.get('admins') === 'true';
// requests: "Заявки" of a closed group, for those who run it.
const requests = params.get('requests') === 'true';
// going is the list of an event: 'sure' unless asked for those who might.
let going = '';

let club;
let cursor = '';

function memberNode(member) {
  const role = el('span', { className: 'club-role' });
  const title = el('div', { className: 'hint' });
  const links = el('span', {});

  function render() {
    role.textContent = clubRoles[member.role];
    title.textContent = member.title ? `Комментарий: ${member.title}` : '';
    title.hidden = !member.title;
    links.replaceChildren(...separated([...appointments(), ...descriptions()].filter(Boolean)));
  }

  // The owner appoints and dismisses the admins and hands the club over to
  // one of them; nobody touches the owner.
  function appointments() {
    if (club.role !== 'owner' || member.role === 'owner') return [];

    const appoint = member.role === 'member';
    return [
      el('a', {
        textContent: appoint ? 'Назначить администратором' : 'Снять полномочия администратора',
        onclick: () => setAdmin(appoint),
      }),
      !appoint && el('a', { textContent: 'Назначить владельцем', onclick: () => confirmTransfer(member.user) }),
    ];
  }

  // Those who run the club describe each other, the owner too.
  function descriptions() {
    if (!manages(club.role) || !manages(member.role)) return [];

    return [
      el('a', { textContent: 'Изменить комментарий', onclick: () => editTitle(member, render) }),
      el('a', { textContent: member.hidden ? 'Скрыт: Да' : 'Скрыт: Нет', onclick: toggleHidden }),
    ];
  }

  async function toggleHidden() {
    hideError($('error'));

    try {
      await api.describeAdmin(clubRef(club), member.user.nickname, { hidden: !member.hidden });
      member.hidden = !member.hidden;
      render();
      notice(member.hidden
        ? `Теперь ${fullName(member.user)} будет показываться как обычный подписчик всем, кроме других администраторов`
        : `Теперь все будут знать, что ${fullName(member.user)} — администратор.`);
    } catch (e) {
      showError($('error'), e);
    }
  }

  async function setAdmin(appoint) {
    hideError($('error'));

    try {
      if (appoint) await api.appointAdmin(clubRef(club), member.user.nickname);
      else await api.dismissAdmin(clubRef(club), member.user.nickname);
      member.role = appoint ? 'admin' : 'member';
      // A dismissed admin keeps neither the title nor the hiding.
      if (!appoint) Object.assign(member, { title: '', hidden: false });
      render();
    } catch (e) {
      showError($('error'), e);
    }
  }

  render();

  return personRow(member.user, el('div', { className: 'hint' }, role), title, el('div', {}, links));
}

// requestNode is one who asked to join: "Принять заявку" lets them in,
// "Отклонить заявку" does not; either way the row goes.
function requestNode(user) {
  const row = personRow(user, el('div', {}, ...separated([
    el('a', { textContent: 'Принять заявку', onclick: () => decide(api.acceptClubRequest, 1) }),
    el('a', { textContent: 'Отклонить заявку', onclick: () => decide(api.rejectClubRequest, 0) }),
  ])));

  async function decide(act, joined) {
    hideError($('error'));

    try {
      await act(clubRef(club), user.nickname);
      row.remove();
      club.requests -= 1;
      club.members += joined;
      $('summary').textContent = requestsCount(club.requests);
      tabs();
    } catch (e) {
      showError($('error'), e);
    }
  }

  return row;
}

async function loadRequests() {
  hideError($('error'));

  try {
    const page = await api.clubRequests(ref, { cursor });

    club.requests = page.total;
    $('summary').textContent = requestsCount(page.total);
    $('summary').hidden = false;
    $('people').append(...page.people.map(requestNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    showError($('error'), e);
  }
}

const manages = (role) => role === 'admin' || role === 'owner';

function notice(text) {
  $('notice').textContent = text;
  $('notice').hidden = false;
}

// editTitle is openvk's "Изменить комментарий к администратору": the few
// words under the admin's name; empty takes them away.
function editTitle(member, done) {
  const modal = openModal('Изменить комментарий к администратору', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const input = el('input', { type: 'text', className: 'modal-input', maxLength: 36, value: member.title });

  modal.body.append(input, error);
  modal.actions.append(
    button('Изменить', null, { type: 'submit' }),
    button('Отмена', modal.close, { secondary: true }),
  );
  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);

    try {
      const title = input.value.trim();

      await api.describeAdmin(clubRef(club), member.user.nickname, { title });
      member.title = title;
      modal.close();
      done();
      notice(title ? 'Комментарий к администратору изменён' : 'Комментарий к администратору удален');
    } catch (e) {
      showError(error, e);
    }
  });
  input.focus();
}

// confirmTransfer is openvk's "Передача прав владельца". openvk also asks
// for the password; there are no passwords here, sign-in is by email.
function confirmTransfer(user) {
  const modal = openModal('Передача прав владельца');
  const error = el('div', { className: 'error', hidden: true });

  modal.body.append(
    el('p', {}, `Внимание! Вы передаёте права владельца пользователю ${fullName(user)}. Это действие необратимо. ` +
      'После передачи вы останетесь администратором, но сможете легко перестать им быть.'),
    error,
  );
  modal.actions.append(
    button('Передать', async (event) => {
      event.target.disabled = true;
      hideError(error);

      try {
        await api.transferClub(clubRef(club), user.nickname);
        modal.close();
        transferred(user);
      } catch (e) {
        showError(error, e);
        event.target.disabled = false;
      }
    }),
    button('Отмена', modal.close, { secondary: true }),
  );
}

// transferred shows what openvk says and the list again, with the roles
// as they now are.
function transferred(user) {
  club.role = 'admin';
  notice(`Новый владелец (${fullName(user)}) успешно назначен в сообщество ${club.name}. ` +
    'Вам выданы права администратора в сообществе.');
  $('people').replaceChildren();
  cursor = '';
  loadMore();
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.clubMembers(ref, { cursor, admins, going });

    $('people').append(...page.members.map(memberNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('people').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

// tab is one list of the page: query picks it, active tells whether it
// is the one shown.
function tab(id, query, text, active) {
  const link = $(id);
  link.textContent = text;
  link.href = `/club/${encodeURIComponent(clubRef(club))}/members${query}`;
  link.classList.toggle('active', active);
  link.hidden = false;
}

// tabs are "Участники" and "Руководство" or, of an event, "Точно придут",
// "Возможно придут" and "Руководство".
function tabs() {
  if (club.kind !== 'event') {
    tab('tab-all', '', `Участники · ${membersCount(club.members)}`, !admins && !requests);
  } else {
    tab('tab-all', '', `Точно придут · ${club.members - club.maybes}`, !admins && going === 'sure');
    tab('tab-maybe', '?going=maybe', `Возможно придут · ${club.maybes}`, !admins && going === 'maybe');
  }

  tab('tab-admins', '?admins=true', 'Руководство', admins);
  if (club.closed && manages(club.role)) tab('tab-requests', '?requests=true', `Заявки · ${club.requests ?? 0}`, requests);
  $('tabs').hidden = false;
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  club = await api.club(ref);
  const path = '/club/' + encodeURIComponent(clubRef(club));

  document.title = `Участники: ${club.name} | Freenet`;
  $('title').textContent = admins ? `Руководство: ${club.name}` : `Участники: ${club.name}`;
  $('back').href = path;

  if (club.kind === 'event' && !admins) going = params.get('going') === 'maybe' ? 'maybe' : 'sure';
  $('back').textContent = club.kind === 'event' ? '← к встрече' : '← к группе';
  tabs();

  // "Заявки" is for those who run a closed group; the others get the members.
  const load = requests && club.closed && manages(club.role) ? loadRequests : loadMore;

  if (load === loadRequests) $('title').textContent = `Заявки: ${club.name}`;
  $('more').addEventListener('click', load);
  await load();
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
  showError($('error'), e);
}
