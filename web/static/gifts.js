// "Подарки" of openvk, what the pages share: where the gifts of a person
// are, where one is chosen for them, and the block of a person's page.

import { giftImage, giftNames, giftsCount } from './labels.js';
import { el } from './layout.js';

// giftsPath is the list of gifts of user.
export function giftsPath(user) {
  return '/gifts?u=' + encodeURIComponent(user.nickname);
}

// pickPath is "Выбрать подарок" for user.
export function pickPath(user) {
  return '/gifts?act=pick&u=' + encodeURIComponent(user.nickname);
}

// giftPicture is the picture of a gift.
export function giftPicture(gift) {
  return el('img', { className: 'gift-pic', src: giftImage(gift), alt: giftNames[gift] ?? 'Подарок', loading: 'lazy' });
}

// giftsBlock is openvk's block "Подарки" of a page: how many, "Отправить
// подарок" for who is signed in, "Все" and the newest.
export function giftsBlock(user, page, canSend) {
  const links = [el('a', { href: giftsPath(user), textContent: 'Все' })];
  if (canSend) links.unshift(el('a', { href: pickPath(user), textContent: 'Отправить подарок' }), ' | ');

  return [
    el('div', { className: 'section-title', textContent: 'Подарки' }),
    el('div', { className: 'subtitle' }, el('span', { textContent: giftsCount(page.total) }), el('span', {}, ...links)),
    el('a', { className: 'gift-row', href: giftsPath(user) }, ...page.gifts.map((g) => giftPicture(g.gift))),
  ];
}
