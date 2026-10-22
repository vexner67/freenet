// "Маркет подарков" at /market, as Telegram's market of collectible
// gifts: those on sale, filtered by gift and traits, cheapest first or as
// chosen, and ?tab=auctions — the running auctions, ending soonest first.
// openvk has nothing like it.

import { api, currentUser, started } from './api.js';
import { giftBackdrops, giftModels, giftNames, giftSymbols, marketSorts } from './labels.js';
import { collectibleTile } from './gifts.js';
import { $, el, hideError, renderChrome, showError } from './layout.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const auctions = params.get('tab') === 'auctions';
const filter = Object.fromEntries(['gift', 'model', 'backdrop', 'symbol', 'sort'].map((name) => [name, params.get(name) ?? '']));

let cursor = '';
const first = started(auctions ? api.giftAuctions() : api.giftMarket(filter));

// options fills a select with [value, text] pairs and chooses the one of
// the address.
function options(select, list, chosen) {
  select.append(...list.map(([value, text]) => el('option', { value, textContent: text, selected: value === chosen })));
}

function showFilters() {
  options($('filter-gift'), Object.entries(giftNames).map(([gift, name]) => [gift, name.toLowerCase()]), filter.gift);
  options($('filter-model'), giftModels, filter.model);
  options($('filter-backdrop'), giftBackdrops, filter.backdrop);
  options($('filter-symbol'), giftSymbols, filter.symbol);
  options($('filter-sort'), marketSorts, filter.sort);
  $('filters').hidden = false;
}

function append(page) {
  $('grid').append(...page.collectibles.map(collectibleTile));
  if (!cursor) $('empty').hidden = page.collectibles.length > 0;
  cursor = page.nextCursor;
  $('more').hidden = !cursor;
}

async function more() {
  hideError($('error'));

  try {
    append(await (auctions ? api.giftAuctions(cursor) : api.giftMarket(filter, cursor)));
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

$(auctions ? 'tab-auctions' : 'tab-market').classList.add('active');
$('tab-mine').hidden = !me;
$('lead').textContent = auctions
  ? tr('Аукционы подарков: тех, что выставили люди, и редких выпусков сайта. Каждая ставка больше предыдущей на 5 %, а ставка в последние 5 минут продлевает аукцион.')
  : tr('Коллекционные подарки, которые продают люди. Свой подарок можно улучшить до коллекционного на странице своих подарков.');
if (auctions) document.title = `${tr('Аукционы подарков')} | Freenet`;
else showFilters();

$('more').addEventListener('click', more);

try {
  append(await first);
} catch (e) {
  showError($('error'), e);
}
