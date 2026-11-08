// "Подарки" of the left menu: the shop of gifts. openvk has none, so it is
// laid out as the page of a person is: beside, "Мой баланс" and "Мои
// подарки"; the main column holds a block for each collection, its gifts
// with their names, prices and what is left of an edition, and under each
// "Купить себе" and "Подарить". Both open the box of openvk
// (giftDialog): bought for oneself the gift comes to one's own page, given
// it goes to the people chosen.

import { api, currentUser } from './api.js';
import { balanceAmount, editionLeft, giftCollections, giftNames, giftOffers, giftPrice, giftsCount, voteRate } from './labels.js';
import { giftDialog, giftPicture, giftThumb, giftsPath, primeOnly } from './gifts.js';
import { $, el, renderChrome, showError } from './layout.js';
import { tr } from './i18n.js';

// The newest of one's gifts shown beside.
const MINE_SHOWN = 6;

// The catalogue and the rate are asked with /me, in one go.
const catalog = api.giftCatalog();
const rate = api.voteRate().then((r) => r.kopecks, () => 0);
const me = await currentUser().catch(() => null);

function showBalance() {
  $('balance').textContent = balanceAmount(me.votes ?? 0);
}

// showMine is "Мои подарки": how many and the newest.
async function showMine() {
  try {
    const page = await api.gifts(me.nickname, '', MINE_SHOWN);

    $('mine-count').textContent = giftsCount(page.total);
    $('mine').replaceChildren(...page.gifts.map(giftThumb));
    $('mine').hidden = page.gifts.length === 0;
  } catch {
    // The rest of the shop works without it.
  }
}

// bought is told of the gifts the box sent: the votes are fewer, and one
// bought for oneself is among one's own.
function bought() {
  showBalance();
  showMine();
}

// item is a gift of the shop: its picture, name and price, what is left of
// a limited edition and what may be done with it. A sold out gift is
// dimmed and says so; one of Prime, for those without it, tells where to
// get Prime.
function item(collection, { gift, price, left }, i) {
  const open = (self) => () => {
    if (collection.prime && !me.prime) {
      primeOnly();
      return;
    }
    giftDialog({ me, gift, price, recipients: self ? [me] : [], shop: true, done: bought });
  };
  const soldOut = left === 0;
  const edition = left === Infinity ? '' : editionLeft(collection.supplies[i], collection.sold[i]);

  return el('div', { className: soldOut ? 'shop-item disabled' : 'shop-item' },
    soldOut ? giftPicture(gift) : el('a', { className: 'shop-item-pic', title: tr('Купить себе'), onclick: open(true) }, giftPicture(gift)),
    el('div', { className: 'shop-item-name', textContent: giftNames[gift] ?? tr('Подарок') }),
    el('div', { className: 'shop-item-price' }, el('b', { textContent: giftPrice(price) })),
    el('div', { className: 'shop-item-left', textContent: edition }),
    soldOut
      ? el('div', { className: 'shop-item-links' })
      : el('div', { className: 'shop-item-links' },
        el('a', { textContent: tr('Купить себе'), onclick: open(true) }), ' | ',
        el('a', { textContent: tr('Подарить'), onclick: open(false) })));
}

// block is a collection: its name, how many gifts and, for Prime, whose
// they are.
function block(collection) {
  const offers = giftOffers(collection);

  return el('div', { className: 'shop-collection' },
    el('div', { className: 'section-title', textContent: giftCollections[collection.id] ?? tr('Подарки') }),
    el('div', { className: 'subtitle' },
      el('span', { textContent: giftsCount(offers.length) }),
      collection.prime ? el('a', { href: '/prime', textContent: tr('Только для Prime') }) : el('span')),
    el('div', { className: 'shop-grid' }, ...offers.map((offer, i) => item(collection, offer, i))));
}

async function show() {
  try {
    const { collections } = await catalog;

    $('collections').replaceChildren(...collections.map(block));
    $('tab-mine').href = giftsPath(me);
    $('mine-all').href = giftsPath(me);
    $('mine').href = giftsPath(me);
    showBalance();
    rate.then((kopecks) => {
      $('rate').textContent = voteRate(kopecks);
      $('rate-row').hidden = !kopecks;
    });
    await showMine();
    $('shop').hidden = false;
  } catch (e) {
    showError($('error'), e);
  }
}

renderChrome(me);

if (!me) {
  location.replace('/');
} else if (!me.profileCompleted) {
  showError($('error'), tr('Сначала заполните свою страницу.'));
} else {
  await show();
}
