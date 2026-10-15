// Search, as openvk's: people ("Люди"), communities ("Группы",
// ?section=groups) or posts ("Записи", ?section=posts), by the words of
// ?q=, a page at a time; a tag ("#поход") finds the posts that say it, and
// the tags of the texts lead here. People are also found by the links of a
// page: ?hometown=, ?city=, ?film=, ?show=, ?book=.

import { api, currentUser } from './api.js';
import { resultsCount } from './labels.js';
import { $, el, isMicroblog, renderChrome, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';
import { personRow } from './people.js';
import { postNode, wallOwnerID } from './posts.js';

const params = new URLSearchParams(location.search);
const query = (params.get('q') ?? '').trim();
const kind = ['groups', 'posts'].includes(params.get('section')) ? params.get('section') : '';
const groups = kind === 'groups';

// filterNames say what a filter of a page's link looks for.
const filterNames = { hometown: 'Родной город', city: 'Город', film: 'Любимые фильмы', show: 'Любимые ТВ-шоу', book: 'Любимые книги' };
const filters = kind ? {} : Object.fromEntries(Object.keys(filterNames)
  .map((key) => [key, (params.get(key) ?? '').trim()])
  .filter(([, value]) => value));
const filtered = Object.keys(filters).length > 0;

const me = await currentUser().catch(() => null);

// A post found is shown as in the news: I edit and delete what I wrote,
// and delete, pin and put away what is on my wall and on the walls of
// the communities I run.
function foundPost(post) {
  const moderates = Boolean(post.moderated);

  return postNode(post, {
    viewer: me,
    moderates,
    onDelete: (post.mine || moderates || wallOwnerID(post) === me.id) && removePost,
    onArchived: (p, node) => { if (p.archived) gone(node, 'Запись перенесена в архив.'); },
    canLike: true,
    onError: (e) => showError($('error'), e),
    onWallOf: me,
    found: query,
  });
}

async function removePost(post, node) {
  hideError($('error'));

  try {
    await api.deletePost(post.id);
  } catch (e) {
    showError($('error'), e);
    return;
  }

  gone(node, 'Запись удалена.');
}

function gone(node, text) {
  node.replaceWith(el('div', { className: 'post-deleted', textContent: text }));
}

// What each section asks and shows; a count, if given, says how many the
// first page says there are.
const sections = {
  '': { title: 'Поиск людей', tab: 'Люди', placeholder: 'Имя, фамилия или никнейм', search: api.search, items: (page) => page.users, row: personRow, none: 'Никого не нашлось.' },
  groups: { title: 'Поиск групп', tab: 'Группы', placeholder: 'Название или адрес группы', search: api.searchClubs, items: (page) => page.clubs, row: (c) => clubRow(c), none: 'Ни одной группы не нашлось.' },
  posts: {
    title: 'Поиск записей',
    tab: 'Записи',
    placeholder: 'Слова записи или #хэштег',
    search: (q, from) => api.searchPosts(q, from, isMicroblog()),
    items: (page) => page.posts,
    row: foundPost,
    // openvk's "no_results_by_this_query" and "results".
    count: (total) => (total === 0 ? 'По данному запросу ничего не найдено.' : resultsCount(total)),
  },
};
const section = sections[kind];

let cursor = '';
let shown = 0;
let total = 0;

async function loadMore() {
  hideError($('error'));

  try {
    const page = await section.search(query, cursor, filters);
    const items = section.items(page);

    $('people').append(...items.map((item) => section.row(item)));
    if (!cursor) total = page.total ?? 0;
    shown += items.length;
    cursor = page.nextCursor;

    $('more').hidden = !cursor;
    $('count').textContent = section.count ? section.count(total) : (shown === 0 ? section.none : `Найдено: ${shown}${cursor ? '+' : ''}`);
    $('count').hidden = false;
  } catch (e) {
    showError($('error'), e);
  }
}

function tab(id, tabKind) {
  const link = $(id);
  const tabParams = new URLSearchParams();
  if (tabKind) tabParams.set('section', tabKind);
  if (query) tabParams.set('q', query);
  link.href = '/search' + (tabParams.size ? '?' + tabParams : '');
  link.classList.toggle('active', tabKind === kind);
}

renderChrome(me);

$('title').textContent = section.title;
tab('tab-people', '');
tab('tab-groups', 'groups');
tab('tab-posts', 'posts');
$('section').value = kind;
$('section').disabled = !kind;
$('q').placeholder = section.placeholder;
// A post is searched for by at most 100 characters.
$('q').maxLength = kind === 'posts' ? 100 : 64;
$('q').value = query;
$('q').focus();
$('intro').hidden = kind !== '';
$('intro-groups').hidden = !groups;
$('intro-posts').hidden = kind !== 'posts';

if (filtered) {
  const [key, value] = Object.entries(filters)[0];
  const rest = new URLSearchParams(query ? { q: query } : {});
  $('filter').replaceChildren(`${filterNames[key]}: `, el('b', { textContent: value }), ' ',
    el('a', { href: '/search' + (rest.size ? '?' + rest : ''), textContent: '(сбросить)' }));
  $('filter').hidden = false;
}

// The posts are searched by those who signed in, as on openvk.
const guest = kind === 'posts' && !me;
$('search-form').hidden = guest;
$('intro-posts').hidden = guest || kind !== 'posts';
$('guest-posts').hidden = !guest;

if ((query || filtered) && !guest) {
  document.title = `${query || Object.values(filters)[0]} — ${section.tab} | Freenet`;
  $('intro').hidden = true;
  $('intro-groups').hidden = true;
  $('intro-posts').hidden = true;
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
