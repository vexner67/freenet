// Search, as openvk's: people ("Люди") or communities ("Группы",
// ?section=groups), by the words of ?q=, a page at a time. People are also
// found by the links of a page: ?hometown=, ?city=, ?film=, ?show=, ?book=.

import { api, currentUser } from './api.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';
import { personRow } from './people.js';

const params = new URLSearchParams(location.search);
const query = (params.get('q') ?? '').trim();
const groups = params.get('section') === 'groups';

// filterNames say what a filter of a page's link looks for.
const filterNames = { hometown: 'Родной город', city: 'Город', film: 'Любимые фильмы', show: 'Любимые ТВ-шоу', book: 'Любимые книги' };
const filters = groups ? {} : Object.fromEntries(Object.keys(filterNames)
  .map((key) => [key, (params.get(key) ?? '').trim()])
  .filter(([, value]) => value));
const filtered = Object.keys(filters).length > 0;

// What each section asks and shows.
const section = groups
  ? { title: 'Поиск групп', placeholder: 'Название или адрес группы', search: api.searchClubs, items: (page) => page.clubs, row: (c) => clubRow(c), none: 'Ни одной группы не нашлось.' }
  : { title: 'Поиск людей', placeholder: 'Имя, фамилия или никнейм', search: api.search, items: (page) => page.users, row: personRow, none: 'Никого не нашлось.' };

let cursor = '';
let shown = 0;

async function loadMore() {
  hideError($('error'));

  try {
    const page = await section.search(query, cursor, filters);
    const items = section.items(page);

    $('people').append(...items.map((item) => section.row(item)));
    shown += items.length;
    cursor = page.nextCursor;

    $('more').hidden = !cursor;
    $('count').textContent = shown === 0 ? section.none : `Найдено: ${shown}${cursor ? '+' : ''}`;
    $('count').hidden = false;
  } catch (e) {
    showError($('error'), e);
  }
}

function tab(id, groupsTab) {
  const link = $(id);
  const tabParams = new URLSearchParams();
  if (groupsTab) tabParams.set('section', 'groups');
  if (query) tabParams.set('q', query);
  link.href = '/search' + (tabParams.size ? '?' + tabParams : '');
  link.classList.toggle('active', groupsTab === groups);
}

renderChrome(await currentUser().catch(() => null));

$('title').textContent = section.title;
tab('tab-people', false);
tab('tab-groups', true);
$('section').value = groups ? 'groups' : '';
$('section').disabled = !groups;
$('q').placeholder = section.placeholder;
$('q').value = query;
$('q').focus();
$('intro').hidden = groups;
$('intro-groups').hidden = !groups;

if (filtered) {
  const [key, value] = Object.entries(filters)[0];
  const rest = new URLSearchParams(query ? { q: query } : {});
  $('filter').replaceChildren(`${filterNames[key]}: `, el('b', { textContent: value }), ' ',
    el('a', { href: '/search' + (rest.size ? '?' + rest : ''), textContent: '(сбросить)' }));
  $('filter').hidden = false;
}

if (query || filtered) {
  document.title = `${query || Object.values(filters)[0]} — ${groups ? 'Группы' : 'Люди'} | Freenet`;
  $('intro').hidden = true;
  $('intro-groups').hidden = true;
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
