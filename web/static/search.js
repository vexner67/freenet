import { api, currentUser } from './api.js';
import { $, renderChrome, showError, hideError } from './layout.js';
import { personRow } from './people.js';

const query = (new URLSearchParams(location.search).get('q') ?? '').trim();

let cursor = '';
let shown = 0;

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.search(query, cursor);

    $('people').append(...page.users.map((user) => personRow(user)));
    shown += page.users.length;
    cursor = page.nextCursor;

    $('more').hidden = !cursor;
    $('count').textContent = shown === 0 ? 'Никого не нашлось.' : `Найдено: ${shown}${cursor ? '+' : ''}`;
    $('count').hidden = false;
  } catch (e) {
    showError($('error'), e);
  }
}

renderChrome(await currentUser().catch(() => null));

$('q').value = query;
$('q').focus();

if (query) {
  document.title = `${query} — Люди | Freenet`;
  $('intro').hidden = true;
  $('more').addEventListener('click', loadMore);
  await loadMore();
}
