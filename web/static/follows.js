import { api, currentUser } from './api.js';
import { $, renderChrome, fullName, showError, hideError } from './layout.js';
import { personRow } from './people.js';

const params = new URLSearchParams(location.search);
const nickname = params.get('u') ?? '';
const type = params.get('type') === 'following' ? 'following' : 'followers';

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

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const owner = await api.profile(nickname);
  const name = fullName(owner);

  $('title').textContent = type === 'followers' ? `Подписчики: ${name}` : `Подписки: ${name}`;
  document.title = $('title').textContent + ' | Freenet';
  $('back').textContent = '← к странице';
  $('back').href = '/' + owner.nickname;

  tab('tab-followers', 'followers', `Подписчики · ${owner.followersCount}`);
  tab('tab-following', 'following', `Подписки · ${owner.followingCount}`);
  $('tabs').hidden = false;

  const subscribed = { male: 'подписан', female: 'подписана' }[owner.gender] ?? 'подписан(а)';
  $('empty').textContent = type === 'followers' ? 'Пока никто не подписан.' : `Пока ни на кого не ${subscribed}.`;
  $('more').addEventListener('click', loadMore);
  await loadMore();
} catch (e) {
  $('title').textContent = 'Страница не найдена';
  showError($('error'), e.status === 400 ? 'Страница не найдена.' : e);
}
