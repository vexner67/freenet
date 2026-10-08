// The members of a club ("Участники") and its admins ("Руководство"), as
// on openvk, the newest first. Its owner makes members admins and back.

import { api, currentUser } from './api.js';
import { clubRoles, membersCount } from './labels.js';
import { $, clubRef, el, renderChrome, showError, hideError } from './layout.js';
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
    links.replaceChildren(...separated(actions()));
  }

  // The owner appoints and dismisses the admins; nobody touches the owner.
  function actions() {
    if (club.role !== 'owner' || member.role === 'owner') return [];

    const appoint = member.role === 'member';
    return [el('a', {
      textContent: appoint ? 'Назначить администратором' : 'Снять полномочия администратора',
      onclick: () => setAdmin(appoint),
    })];
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
