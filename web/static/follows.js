import { api, currentUser, started } from './api.js';
import { $, renderChrome, fullName, showError, hideError } from './layout.js';
import { personRow } from './people.js';

const params = new URLSearchParams(location.search);
const nickname = params.get('u') ?? '';
const types = ['followers', 'following', 'online', 'common'];
const type = types.includes(params.get('type')) ? params.get('type') : 'followers';

let cursor = '';

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api[type](nickname, cursor);

    $('people').append(...page.users.map((user) => personRow(user)));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
    $('empty').hidden = $('people').children.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

function tab(id, kind, text) {
  const link = $(id);
  link.textContent = text;
  link.href = `/follows?u=${encodeURIComponent(nickname)}&type=${kind}`;
  link.classList.toggle('active', kind === type);
}

// The page is asked for with who is signed in, in one round trip.
const wanted = started(api.profile(nickname));
const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const owner = await wanted;
  const name = fullName(owner);

  $('title').textContent = { followers: 'Подписчики', following: 'Подписки', online: 'Подписки онлайн', common: 'Общие подписки' }[type] + ': ' + name;
  document.title = $('title').textContent + ' | Freenet';
  $('back').textContent = '← к странице';
  $('back').href = '/' + owner.nickname;

  tab('tab-followers', 'followers', `Подписчики · ${owner.followersCount}`);
  tab('tab-following', 'following', `Подписки · ${owner.followingCount}`);
  tab('tab-online', 'online', 'Онлайн');
  // As openvk's "Общие друзья", to someone else signed in.
  $('tab-common').hidden = !me || me.id === owner.id;
  tab('tab-common', 'common', 'Общие');
  $('tabs').hidden = false;

  const subscribed = { male: 'подписан', female: 'подписана' }[owner.gender] ?? 'подписан(а)';
  $('empty').textContent = {
    followers: 'Пока никто не подписан.',
    following: `Пока ни на кого не ${subscribed}.`,
    online: 'Никого из подписок нет онлайн.',
    common: 'Общих подписок нет.',
  }[type];
  $('more').addEventListener('click', loadMore);
  await loadMore();
} catch (e) {
  $('title').textContent = 'Страница не найдена';
  showError($('error'), e.status === 400 ? 'Страница не найдена.' : e);
}
