// Search, as openvk's: people ("Люди") or communities ("Группы",
// ?section=groups), by the words of ?q=, a page at a time.

import { api, currentUser } from './api.js';
import { $, renderChrome, showError, hideError } from './layout.js';
import { clubRow } from './clubs.js';
import { personRow } from './people.js';

const params = new URLSearchParams(location.search);
const query = (params.get('q') ?? '').trim();
const groups = params.get('section') === 'groups';

// What each section asks and shows.
const section = groups
  ? { title: 'Поиск групп', placeholder: 'Название или адрес группы', search: api.searchClubs, items: (page) => page.clubs, row: (c) => clubRow(c), none: 'Ни одной группы не нашлось.' }
  : { title: 'Поиск людей', placeholder: 'Имя, фамилия или никнейм', search: api.search, items: (page) => page.users, row: personRow, none: 'Никого не нашлось.' };

let cursor = '';
let shown = 0;

async function loadMore() {
  hideError($('error'));

  try {
    const page = await section.search(query, cursor);
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

if (query) {
  document.title = `${query} — ${groups ? 'Группы' : 'Люди'} | Freenet`;
  $('intro').hidden = true;
  $('intro-groups').hidden = true;
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
