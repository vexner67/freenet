// The members of a club ("Участники") and its admins ("Руководство"), as
// on openvk, the newest first. Its owner makes members admins and back,
// and hands the club over to an admin ("Назначить владельцем"); those who
// run it give each other a title ("Изменить комментарий") and hide each
// other from the rest ("Скрыт").

import { api, currentUser } from './api.js';
import { clubRoles, membersCount, requestsCount } from './labels.js';
import { $, avatarURL, clubRef, el, fullName, renderChrome, showError, hideError, verifiedMark } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/members'.length));
const params = new URLSearchParams(location.search);
const admins = params.get('admins') === 'true';
// requests: "Заявки" of a closed group, for those who run it.
const requests = params.get('requests') === 'true';

let club;
let cursor = '';

function memberRow(user, facts, actions) {
  const path = '/' + encodeURIComponent(user.nickname);
  return el('div', { className: 'list-row' },
    el('a', { className: 'list-cover', href: path }, el('img', { src: avatarURL(user, 'small'), alt: '', loading: 'lazy' })),
    el('div', { className: 'list-info' },
      el('a', { className: 'club-member-name', href: path, textContent: fullName(user) }),
      verifiedMark(user), facts),
    actions,
  );
}

function action(text, onclick) {
  return el('button', { className: 'profile-link', type: 'button', textContent: text, onclick });
}

function memberNode(member) {
  const role = el('td');
  const title = el('td');
  const comment = el('tr', {}, el('th', { textContent: tr('Комментарий:') }), title);
  const facts = el('table', { className: 'info' },
    el('tr', {}, el('th', { textContent: tr('Роль:') }), role), comment);
  const links = el('div', { className: 'list-actions' });

  function render() {
    role.textContent = clubRoles[member.role];
    title.textContent = member.title;
    comment.hidden = !member.title;
    links.replaceChildren(...[...appointments(), ...descriptions()].filter(Boolean));
    links.hidden = !links.childElementCount;
  }

  // The owner appoints and dismisses the admins and hands the club over to
  // one of them; nobody touches the owner.
  function appointments() {
    if (club.role !== 'owner' || member.role === 'owner') return [];

    const appoint = member.role === 'member';
    return [
      action(appoint ? tr('Назначить администратором') : tr('Снять полномочия администратора'), () => setAdmin(appoint)),
      !appoint && action(tr('Назначить владельцем'), () => confirmTransfer(member.user)),
    ];
  }

  // Those who run the club describe each other, the owner too.
  function descriptions() {
    if (!manages(club.role) || !manages(member.role)) return [];

    return [
      action(tr('Изменить комментарий'), () => editTitle(member, render)),
      action(member.hidden ? tr('Скрыт: Да') : tr('Скрыт: Нет'), toggleHidden),
    ];
  }

  async function toggleHidden() {
    hideError($('error'));

    try {
      await api.describeAdmin(clubRef(club), member.user.nickname, { hidden: !member.hidden });
      member.hidden = !member.hidden;
      render();
      notice(member.hidden
        ? tr('Теперь {0} будет показываться как обычный подписчик всем, кроме других администраторов', fullName(member.user))
        : tr('Теперь все будут знать, что {0} — администратор.', fullName(member.user)));
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

  return memberRow(member.user, facts, links);
}

// requestNode is one who asked to join: "Принять заявку" lets them in,
// "Отклонить заявку" does not; either way the row goes.
function requestNode(user) {
  const row = memberRow(user, null, el('div', { className: 'list-actions' },
    action(tr('Принять заявку'), () => decide(api.acceptClubRequest, 1)),
    action(tr('Отклонить заявку'), () => decide(api.rejectClubRequest, 0)),
  ));

  async function decide(act, joined) {
    hideError($('error'));

    try {
      await act(clubRef(club), user.nickname);
      row.remove();
      club.requests -= 1;
      club.members += joined;
      $('summary').textContent = requestsCount(club.requests);
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
  const modal = openModal(tr('Изменить комментарий к администратору'), { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const input = el('input', { type: 'text', className: 'modal-input', maxLength: 36, value: member.title });

  modal.body.append(input, error);
  modal.actions.append(
    button(tr('Изменить'), null, { type: 'submit' }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);

    try {
      const title = input.value.trim();

      await api.describeAdmin(clubRef(club), member.user.nickname, { title, baseTitle: member.title ?? '' });
      member.title = title;
      modal.close();
      done();
      notice(title ? tr('Комментарий к администратору изменён') : tr('Комментарий к администратору удален'));
    } catch (e) {
      showError(error, e);
    }
  });
  input.focus();
}

// confirmTransfer is openvk's "Передача прав владельца". openvk also asks
// for the password; there are no passwords here, sign-in is by email.
function confirmTransfer(user) {
  const modal = openModal(tr('Передача прав владельца'));
  const error = el('div', { className: 'error', hidden: true });

  modal.body.append(
    el('p', {}, tr('Внимание! Вы передаёте права владельца пользователю {0}. Это действие необратимо. ', fullName(user)) +
      tr('После передачи вы останетесь администратором, но сможете легко перестать им быть.')),
    error,
  );
  modal.actions.append(
    button(tr('Передать'), async (event) => {
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
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

// transferred shows what openvk says and the list again, with the roles
// as they now are.
function transferred(user) {
  club.role = 'admin';
  notice(tr('Новый владелец ({0}) успешно назначен в сообщество {1}. ', fullName(user), club.name) +
    tr('Вам выданы права администратора в сообществе.'));
  $('people').replaceChildren();
  cursor = '';
  loadMore();
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.clubMembers(ref, { cursor, admins });

    if (!cursor) {
      $('summary').textContent = membersCount(page.total);
      $('summary').hidden = false;
    }
    $('people').append(...page.members.map(memberNode));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('people').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

function tabs() {
  const path = '/club/' + encodeURIComponent(clubRef(club));
  const manager = manages(club.role);
  const requestView = requests && club.closed && manager;
  $('tab-main').href = path + '/edit';
  $('tab-main').hidden = !manager;
  $('tab-backdrop').href = path + '/backdrop';
  $('tab-backdrop').hidden = !manager;
  $('tab-all').href = path + '/members';
  $('tab-all').classList.toggle('active', !requestView);
  $('tab-requests').href = path + '/members?requests=true';
  $('tab-requests').classList.toggle('active', requestView);
  $('tab-requests').hidden = !(club.closed && manager);
  $('tabs').hidden = false;
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  club = await api.club(ref);
  const path = '/club/' + encodeURIComponent(clubRef(club));

  document.title = tr('Участники: {0} | Freenet', club.name);
  const load = requests && club.closed && manages(club.role) ? loadRequests : loadMore;
  const requestView = load === loadRequests;
  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ' » ',
    requestView ? tr('Заявки') : tr('Участники'));
  const filter = $('member-filter');
  filter.href = path + (admins ? '/members' : '/members?admins=true');
  filter.textContent = admins ? tr('Все участники') : tr('Только администраторы');
  filter.hidden = requestView;
  tabs();
  $('more').addEventListener('click', load);
  await load();
} catch (e) {
  $('title').textContent = e.status === 404 ? tr('Группа не найдена') : tr('Ошибка');
  showError($('error'), e);
}
