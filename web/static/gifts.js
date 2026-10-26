// "Подарки" of openvk, what the pages share: where the gifts of a person
// are, where one is chosen for them, and the block of a person's page.

import { auctionLeft, backdropGradient, collectibleImage, collectibleName, giftCollections, giftCosts, giftImage, giftNames, giftPrice, giftsCount, symbolGlyph, votesCount } from './labels.js';
import { el } from './layout.js';
import { tr } from './i18n.js';

// giftsPath is the list of gifts of user.
export function giftsPath(user) {
  return '/gifts?u=' + encodeURIComponent(user.nickname);
}

// pickPath is "Выбрать подарок" for user.
export function pickPath(user) {
  return '/gifts?act=pick&u=' + encodeURIComponent(user.nickname);
}

// menuPath is a collection of "Выбрать подарок" for user, all its gifts.
export function menuPath(user, collection) {
  return `/gifts?act=menu&pack=${encodeURIComponent(collection)}&u=${encodeURIComponent(user.nickname)}`;
}

// giftPicture is the picture of a gift.
export function giftPicture(gift) {
  return el('img', { className: 'gift-pic', src: giftImage(gift), alt: giftNames[gift] ?? tr('Подарок'), loading: 'lazy' });
}

// giftThumb is a sent gift in a row: a collectible as its card, on its
// backdrop and pattern.
export function giftThumb(gift) {
  if (!gift.number) return giftPicture(gift.gift);

  return collectibleCard(gift, 'thumb');
}

// giftsBlock is openvk's block "Подарки" of a page: how many, "Отправить
// подарок" for who is signed in — on one's own page "Купить подарок", one
// for oneself — "Все" and the newest.
export function giftsBlock(user, page, canSend, mine) {
  const links = [el('a', { href: giftsPath(user), textContent: tr('Все') })];
  if (mine) links.unshift(el('a', { href: pickPath(user), textContent: tr('Купить подарок') }), ' | ');
  else if (canSend) links.unshift(el('a', { href: pickPath(user), textContent: tr('Отправить подарок') }), ' | ');

  return [
    el('div', { className: 'section-title', textContent: tr('Подарки') }),
    el('div', { className: 'subtitle' }, el('span', { textContent: giftsCount(page.total) }), el('span', {}, ...links)),
    el('a', { className: 'gift-row', href: giftsPath(user) }, ...page.gifts.map(giftThumb)),
  ];
}

// giftRow is a collection of "Выбрать подарок", as openvk's gift_cat_row:
// its name, "Показать больше" to all its gifts when more is given, and the
// gifts — the first limit of them — each with its price on pointing;
// pick is called with the gift chosen and its price.
export function giftRow(collection, { more = '', limit = Infinity } = {}, pick) {
  return el('div', { className: 'gift-cat-row' },
    el('div', { className: 'gift-cat-info' },
      el('h4', { className: 'gift-cat-header', textContent: giftCollections[collection.id] ?? tr('Подарки') }),
      collection.prime ? el('a', { className: 'gift-cat-prime', href: '/prime', textContent: tr('Только для Prime') }) : null,
      more ? el('a', { className: 'button', href: more, textContent: tr('Показать больше') }) : null),
    el('div', { className: 'gift-grid' }, ...giftCosts(collection).slice(0, limit).map(({ gift, price }) => el('a', {
      className: 'gift-sel',
      title: giftNames[gift] ?? tr('Подарок'),
      onclick: () => pick(gift, price),
    },
    giftPicture(gift),
    el('strong', { className: price > 0 ? 'gift-price' : 'gift-price free', textContent: giftPrice(price) }),
    ))),
  );
}

// giftPath is the page of a collectible gift.
export function giftPath(id) {
  return '/gift/' + encodeURIComponent(id);
}

// collectibleCard is a collectible as Telegram draws one: its backdrop,
// the symbol of its pattern scattered over it and the gift in its model
// on top; size is 'thumb' (in the row of a page), 'small' or 'large'. The colours are the gift's own, so
// they are painted through the CSSOM rather than by the theme.
export function collectibleCard(gift, size = 'small') {
  const pattern = el('div', { className: 'collectible-pattern' },
    ...Array.from({ length: 12 }, () => el('span', { textContent: symbolGlyph(gift.symbol) })));
  const card = el('div', { className: `collectible ${size}` },
    pattern,
    el('img', { className: 'collectible-model', src: collectibleImage(gift.gift, gift.model), alt: collectibleName(gift), loading: 'lazy' }),
    el('span', { className: 'collectible-number', textContent: `#${gift.number}` }));

  card.style.background = backdropGradient(gift.backdrop);
  pattern.style.color = gift.backdrop === 'night' || gift.backdrop === 'black' ? 'rgba(255, 255, 255, 0.16)' : 'rgba(255, 255, 255, 0.38)';

  return card;
}

// collectibleTile is a collectible of the market or of the auctions: the
// card, its name and its price or the bid and what is left of the time.
export function collectibleTile({ gift, auction }) {
  const price = auction
    ? el('span', { className: 'collectible-price' },
      el('b', { textContent: votesCount(auction.topBid || auction.start) }),
      el('span', { className: 'hint', textContent: ` · ${auctionLeft(auction.endsAt)}` }))
    : el('b', { className: 'collectible-price', textContent: votesCount(gift.salePrice) });

  return el('a', { className: 'collectible-tile', href: giftPath(gift.id) },
    collectibleCard(gift),
    el('span', { className: 'collectible-name', textContent: collectibleName(gift) }),
    price);
}
