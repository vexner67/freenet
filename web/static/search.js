// Search, as openvk's: people ("Люди"), communities ("Группы",
// ?section=groups) or posts ("Записи", ?section=posts), by the words of
// ?q=, a page at a time; a tag ("#поход") finds the posts that say it, and
// the tags of the texts lead here. People are also found by the links of a
// page: ?hometown=, ?city=, ?film=, ?show=, ?book=. As openvk's page: the
// field with where to look and "Поиск", the grey bar of sections and
// "Найдено …" over the list. Results come as the words are typed, the
// sections change without a new page, and the next page comes by itself
// when the end of the list is near.

import { api, currentUser } from './api.js';
import { resultsCount } from './labels.js';
import { $, el, isMicroblog, renderChrome, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';
import { personRow } from './people.js';
import { postNode, wallOwnerID } from './posts.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
let query = (params.get('q') ?? '').trim();
let kind = ['groups', 'posts'].includes(params.get('section')) ? params.get('section') : '';

// filterNames say what a filter of a page's link looks for.
const filterNames = { hometown: tr('Родной город'), city: tr('Город'), film: tr('Любимые фильмы'), show: tr('Любимые ТВ-шоу'), book: tr('Любимые книги') };
let filters = kind ? {} : Object.fromEntries(Object.keys(filterNames)
  .map((key) => [key, (params.get(key) ?? '').trim()])
  .filter(([, value]) => value));
const isFiltered = () => Object.keys(filters).length > 0;
// How long the field stays quiet before the words are searched for.
const typingPause = 280;

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
    onArchived: (p, node) => { if (p.archived) gone(node, tr('Запись перенесена в архив.')); },
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

  gone(node, tr('Запись удалена.'));
}

function gone(node, text) {
  node.replaceWith(el('div', { className: 'post-deleted', textContent: text }));
}

// What each section asks and shows; a count, if given, says how many the
// first page says there are.
const sections = {
  '': { title: tr('Поиск людей'), tab: tr('Люди'), placeholder: tr('Имя, фамилия или никнейм'), search: api.search, items: (page) => page.users, row: personRow, none: tr('Никого не нашлось.') },
  groups: { title: tr('Поиск групп'), tab: tr('Группы'), placeholder: tr('Название или адрес группы'), search: api.searchClubs, items: (page) => page.clubs, row: (c) => clubRow(c), none: tr('Ни одной группы не нашлось.') },
  posts: {
    title: tr('Поиск записей'),
    tab: tr('Записи'),
    placeholder: tr('Слова записи или #хэштег'),
    search: (q, from) => api.searchPosts(q, from, isMicroblog()),
    items: (page) => page.posts,
    row: foundPost,
    // openvk's "no_results_by_this_query" and "results".
    count: (total) => (total === 0 ? tr('По данному запросу ничего не найдено.') : resultsCount(total)),
  },
};
const section = () => sections[kind];

let cursor = '';
let shown = 0;
let total = 0;
let loading = false;
// Only the last search is answered; the words have changed for the others.
let asked = 0;
let typing = 0;

function say(text) {
  $('count').textContent = text;
}

function showPage(page, items) {
  $('people').append(...items.map((item) => section().row(item)));
  if (!cursor) total = page.total ?? 0;
  shown += items.length;
  cursor = page.nextCursor;
  $('more').hidden = !cursor;
  say(section().count ? section().count(total) : (shown === 0 ? section().none : tr('Найдено: {0}{1}', shown, cursor ? '+' : '')));
  if (shown === 0) $('people').append(el('p', { className: 'search-none', textContent: tr('Попробуйте другие слова или загляните в другой раздел.') }));
}

// loadMore adds the next page of what is found.
async function loadMore() {
  if (loading || !cursor) return;

  const mine = asked;

  loading = true;
  try {
    const page = await section().search(query, cursor, filters);

    if (mine === asked) showPage(page, section().items(page));
  } catch (e) {
    showError($('error'), e);
  } finally {
    loading = false;
  }
}

// find searches anew; what is on the screen stays, dimmed, until the answer.
async function find() {
  asked += 1;
  const mine = asked;
  const active = Boolean(query || isFiltered()) && !guest();

  hideError($('error'));
  showIntro(!active);
  if (!active) {
    $('people').replaceChildren();
    $('more').hidden = true;
    say('');
    return;
  }

  $('people').classList.add('searching');
  try {
    const page = await section().search(query, '', filters);
    if (mine !== asked) return;

    $('people').replaceChildren();
    cursor = '';
    shown = 0;
    showPage(page, section().items(page));
  } catch (e) {
    if (mine === asked) showError($('error'), e);
  } finally {
    if (mine === asked) $('people').classList.remove('searching');
  }
}

// guest: the posts are searched by those who signed in, as on openvk.
const guest = () => kind === 'posts' && !me;

function showIntro(shownNow) {
  $('intro').hidden = !shownNow || kind !== '';
  $('intro-groups').hidden = !shownNow || kind !== 'groups';
  $('intro-posts').hidden = !shownNow || kind !== 'posts' || guest();
  $('guest-posts').hidden = !guest();
}

function address(tabKind = kind) {
  const list = new URLSearchParams();

  if (tabKind) list.set('section', tabKind);
  if (query) list.set('q', query);
  if (tabKind === '') for (const [key, value] of Object.entries(filters)) list.set(key, value);

  return '/search' + (list.size ? '?' + list : '');
}

function showTabs() {
  for (const [id, tabKind] of [['tab-people', ''], ['tab-groups', 'groups'], ['tab-posts', 'posts']]) {
    $(id).href = address(tabKind);
    $(id).classList.toggle('active', tabKind === kind);
  }
}

// showSection dresses the page for its section.
function showSection() {
  $('title').textContent = section().title;
  $('section').value = kind;
  $('q').placeholder = section().placeholder;
  // A post is searched for by at most 100 characters.
  $('q').maxLength = kind === 'posts' ? 100 : 64;
  $('search-form').hidden = guest();
  $('filter').hidden = kind !== '' || !isFiltered();
}

function showWords() {
  document.title = query || isFiltered() ? `${query || Object.values(filters)[0]} — ${section().tab} | Freenet` : `${section().title} | Freenet`;
  showTabs();
  history.replaceState(null, '', address());
}

function searchFor(words) {
  clearTimeout(typing);
  query = words.trim();
  $('q').value = query;
  showWords();
  find();
}

// switchTo shows another section for the same words: a tab or the list
// by the field.
function switchTo(tabKind) {
  clearTimeout(typing);
  kind = tabKind;
  if (kind) filters = {};
  showSection();
  showWords();
  find();
  $('q').focus();
}

for (const [id, tabKind] of [['tab-people', ''], ['tab-groups', 'groups'], ['tab-posts', 'posts']]) {
  $(id).addEventListener('click', (event) => {
    if (event.button || event.metaKey || event.ctrlKey || event.shiftKey) return;
    event.preventDefault();
    switchTo(tabKind);
  });
}
$('section').addEventListener('change', () => switchTo($('section').value));

$('q').addEventListener('input', () => {
  query = $('q').value.trim();
  clearTimeout(typing);
  typing = setTimeout(() => {
    showWords();
    find();
  }, typingPause);
});

$('search-form').addEventListener('submit', (event) => {
  event.preventDefault();
  searchFor($('q').value);
});

$('more').addEventListener('click', loadMore);

// The examples of the hints are one press from being searched.
for (const example of document.querySelectorAll('.search-example')) {
  example.addEventListener('click', () => searchFor(example.textContent));
}

document.addEventListener('keydown', (event) => {
  if (event.key === '/' && !event.target.closest?.('input, textarea, select')) {
    event.preventDefault();
    $('q').focus();
    $('q').select();
  }
  if (event.key === 'Escape' && event.target === $('q') && query) searchFor('');
});

// The next page comes before the end of the list is reached.
new IntersectionObserver((seen) => {
  if (seen.some((entry) => entry.isIntersecting)) loadMore();
}, { rootMargin: '300px' }).observe($('more'));

renderChrome(me);

if (isFiltered()) {
  const [key, value] = Object.entries(filters)[0];
  const rest = new URLSearchParams(query ? { q: query } : {});
  $('filter').replaceChildren(`${filterNames[key]}: `, el('b', { textContent: value }), ' ',
    el('a', { href: '/search' + (rest.size ? '?' + rest : ''), textContent: tr('(сбросить)') }));
}

$('q').value = query;
showSection();
showWords();
$('q').focus();
await find();
