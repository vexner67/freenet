// "Подарки" of openvk, what the pages share: where the gifts of a person
// are, where one is chosen for them, and the block of a person's page.

import { giftCollections, giftCosts, giftImage, giftNames, giftPrice, giftsCount } from './labels.js';
import { el } from './layout.js';

// giftsPath is the list of gifts of user.
export function giftsPath(user) {
  return '/gifts?u=' + encodeURIComponent(user.nickname);
}

// pickPath is "Выбрать подарок" for user.
export function pickPath(user) {
  return '/gifts?act=pick&u=' + encodeURIComponent(user.nickname);
}

// SHOP_GIFTS are the gifts of the "Магазин", which one buys for oneself.
export const SHOP_GIFTS = '/shop?act=gifts';

// giftPicture is the picture of a gift.
export function giftPicture(gift) {
  return el('img', { className: 'gift-pic', src: giftImage(gift), alt: giftNames[gift] ?? 'Подарок', loading: 'lazy' });
}

// giftsBlock is openvk's block "Подарки" of a page: how many, "Отправить
// подарок" for who is signed in — on one's own page "Купить подарок" of the
// shop — "Все" and the newest.
export function giftsBlock(user, page, canSend, mine) {
  const links = [el('a', { href: giftsPath(user), textContent: 'Все' })];
  if (mine) links.unshift(el('a', { href: SHOP_GIFTS, textContent: 'Купить подарок' }), ' | ');
  else if (canSend) links.unshift(el('a', { href: pickPath(user), textContent: 'Отправить подарок' }), ' | ');

  return [
    el('div', { className: 'section-title', textContent: 'Подарки' }),
    el('div', { className: 'subtitle' }, el('span', { textContent: giftsCount(page.total) }), el('span', {}, ...links)),
    el('a', { className: 'gift-row', href: giftsPath(user) }, ...page.gifts.map((g) => giftPicture(g.gift))),
  ];
}

// giftShelves are the collections of the catalogue, each with its gifts
// and what each costs; pick is called with the gift chosen and its price.
export function giftShelves(collections, pick) {
  return collections.map((collection) => el('div', { className: 'gift-collection' },
    el('h4', { className: 'gift-collection-title', textContent: giftCollections[collection.id] ?? 'Подарки' }),
    el('div', { className: 'gift-grid' }, ...giftCosts(collection).map(({ gift, price }) => el('a', { className: 'gift-sel', onclick: () => pick(gift, price) },
      giftPicture(gift),
      el('strong', { className: 'gift-price', textContent: giftPrice(price) }),
    ))),
  ));
}
