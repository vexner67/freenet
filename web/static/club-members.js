// The members of a club ("Участники") and its admins ("Руководство"), as
// on openvk, the newest first. Its owner makes members admins and back,
// and hands the club over to an admin ("Назначить владельцем").

import { api, currentUser } from './api.js';
import { clubRoles, membersCount } from './labels.js';
import { $, clubRef, el, fullName, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { personRow } from './people.js';
import { separated } from './posts.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/members'.length));
const admins = new URLSearchParams(location.search).get('admins') === 'true';

let club;
let cursor = '';

function memberNode(member) {
  const role = el('span', { className: 'club-role' });
  const links = el('span', {});

  function render() {
    role.textContent = clubRoles[member.role];
    links.replaceChildren(...separated(actions().filter(Boolean)));
  }

  // The owner appoints and dismisses the admins and hands the club over to
  // one of them; nobody touches the owner.
  function actions() {
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

  async function setAdmin(appoint) {
    hideError($('error'));

    try {
      if (appoint) await api.appointAdmin(clubRef(club), member.user.nickname);
      else await api.dismissAdmin(clubRef(club), member.user.nickname);
      member.role = appoint ? 'admin' : 'member';
      render();
    } catch (e) {
      showError($('error'), e);
    }
  }

  render();

  return personRow(member.user, el('div', { className: 'hint' }, role), el('div', {}, links));
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
  $('notice').textContent = `Новый владелец (${fullName(user)}) успешно назначен в сообщество ${club.name}. ` +
    'Вам выданы права администратора в сообществе.';
  $('notice').hidden = false;
  $('people').replaceChildren();
  cursor = '';
  loadMore();
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.clubMembers(ref, { cursor, admins });

    $('people').append(...page.members.map(memberNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('people').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

function tab(id, admin, text) {
  const link = $(id);
  link.textContent = text;
  link.href = `/club/${encodeURIComponent(clubRef(club))}/members${admin ? '?admins=true' : ''}`;
  link.classList.toggle('active', admin === admins);
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  club = await api.club(ref);
  const path = '/club/' + encodeURIComponent(clubRef(club));

  document.title = `Участники: ${club.name} | Freenet`;
  $('title').textContent = admins ? `Руководство: ${club.name}` : `Участники: ${club.name}`;
  $('back').textContent = '← к группе';
  $('back').href = path;

  tab('tab-all', false, `Участники · ${membersCount(club.members)}`);
  tab('tab-admins', true, 'Руководство');
  $('tabs').hidden = false;

  $('more').addEventListener('click', loadMore);
  await loadMore();
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
  showError($('error'), e);
}
