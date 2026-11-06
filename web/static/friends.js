// "Друзья" of openvk (/friends?u=<nickname>&section=…): "Все друзья",
// "Друзья онлайн", on one's own page "Заявки" with "Входящие" and
// "Исходящие" under it, on another's "Общие друзья".
// Friends follow each other; a follow not followed back is a request. On
// one's own page each person has the links of openvk: a friend "Отправить
// сообщение" and "Удалить из друзей", a request "Принять заявку" and
// "Оставить в подписчиках", an outgoing
// request "Отменить заявку"; the row goes once done.

import { api, currentUser, started } from './api.js';
import { friendsSummary } from './labels.js';
import { $, el, renderChrome, fullName, showError, hideError, setFriendRequests } from './layout.js';
import { personRow } from './people.js';
import { separated } from './posts.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const sections = ['all', 'online', 'common', 'requests', 'outgoing'];
const section = sections.includes(params.get('section')) ? params.get('section') : 'all';
const requests = ['requests', 'outgoing'];

let nickname = params.get('u') ?? '';
let cursor = '';
let total = 0;
let own = false;

function href(kind) {
  const query = new URLSearchParams();
  if (!own) query.set('u', nickname);
  if (kind !== 'all') query.set('section', kind);
  const search = query.toString();

  return '/friends' + (search ? '?' + search : '');
}

function showSummary() {
  $('summary').textContent = friendsSummary(section, total, own);
  $('summary').hidden = false;
}

// done takes the row away and says what happened, as openvk's messages.
function done(row, message) {
  row.remove();
  total -= 1;
  showSummary();
  $('notice').textContent = message;
  $('notice').hidden = false;
  $('empty').hidden = $('people').children.length > 0;
}

// actions are the links of one's own list for the person.
function actions(user, row) {
  const act = (text, call, message) => el('a', {
    textContent: text,
    onclick: async () => {
      hideError($('error'));
      try {
        await call(user.nickname);
        if (section === 'requests') {
          setFriendRequests(total - 1);
          requestsTab(total - 1);
        }
        done(row(), message);
      } catch (e) {
        showError($('error'), e);
      }
    },
  });

  switch (section) {
    case 'all':
    case 'online':
      return [
        el('a', { href: '/im/' + encodeURIComponent(user.nickname), textContent: tr('Отправить сообщение') }),
        act(tr('Удалить из друзей'), api.unfollow, tr('Вы удалили пользователя из списка своих друзей.')),
      ];
    case 'requests':
      return [
        act(tr('Принять заявку'), api.follow, tr('Теперь вы друзья.')),
        act(tr('Оставить в подписчиках'), api.declineFriend, tr('Вы оставили пользователя в подписчиках.')),
      ];
    case 'outgoing':
      return [act(tr('Отменить заявку'), api.unfollow, tr('Вы отменили заявку.'))];
    default:
      return [];
  }
}

function personNode(user) {
  if (!own) return personRow(user);

  let row = null;
  row = personRow(user, el('div', {}, ...separated(actions(user, () => row))));

  return row;
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.friends(nickname, section, cursor);

    $('people').append(...page.users.map(personNode));
    cursor = page.nextCursor;
    total = page.total;
    showSummary();
    $('more').hidden = !cursor;
    $('empty').hidden = $('people').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

// requestsTab is "Заявки (2)", as openvk's, or "Заявки" with none.
function requestsTab(n) {
  $('tab-requests').textContent = n > 0 ? tr('Заявки ({0})', n) : tr('Заявки');
}

function tab(id, kind, text) {
  const link = $(id);
  link.textContent = text;
  link.href = href(kind);
  link.classList.toggle('active', kind === section || (kind === 'requests' && requests.includes(section)));
}

// Without ?u= the page is one's own, as openvk's "Мои Друзья"; with it the
// page is asked for with who is signed in, in one round trip.
const wanted = nickname && started(api.profile(nickname));
const me = await currentUser().catch(() => null);
if (!nickname && me) nickname = me.nickname;
renderChrome(me);

try {
  const owner = await (wanted || api.profile(nickname));
  const name = fullName(owner);
  own = Boolean(me) && me.id === owner.id;

  $('title').textContent = own ? tr('Друзья') : tr('Друзья: {0}', name);
  document.title = $('title').textContent + ' | Freenet';
  $('back').textContent = tr('← к странице');
  $('back').href = '/' + owner.nickname;

  tab('tab-all', 'all', tr('Все'));
  tab('tab-online', 'online', tr('Онлайн'));
  $('tab-requests').hidden = !own;
  tab('tab-requests', 'requests', '');
  requestsTab(owner.requestsCount);
  $('tab-common').hidden = !me || own;
  tab('tab-common', 'common', tr('Общие друзья'));
  $('tabs').hidden = false;

  $('sections').hidden = !own || !requests.includes(section);
  for (const kind of requests) {
    const link = $(`section-${kind}`);
    link.href = href(kind);
    link.classList.toggle('active', kind === section);
  }

  // Another's requests are not to be seen: their friends instead.
  if (!own && requests.includes(section)) location.replace(href('all'));

  $('more').addEventListener('click', loadMore);
  await loadMore();
} catch (e) {
  $('title').textContent = tr('Страница не найдена');
  showError($('error'), e.status === 400 ? tr('Страница не найдена.') : e);
}
