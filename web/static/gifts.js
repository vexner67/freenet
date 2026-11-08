// "Подарки" of openvk, what the pages share: where the gifts of a person
// are, where one is chosen for them, and the block of a person's page.

import { api } from './api.js';
import { auctionLeft, backdropGradient, balanceAmount, collectibleImage, collectibleName, errorText, giftCollections, giftImage, giftNames, giftOffers, giftPrice, giftTotal, giftsCount, giftsLeft, recipientChoices, symbolGlyph, voteRate, votesCount } from './labels.js';
import { avatarURL, el, fullName, hideError, showError } from './layout.js';
import { button, openModal } from './modal.js';
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

// confirmPath is openvk's "Подтверждение" of a gift of a collection for
// user: the page that sends it without the box.
export function confirmPath(user, collection, gift) {
  return `/gifts?act=confirm&pack=${encodeURIComponent(collection)}&elid=${encodeURIComponent(gift)}&u=${encodeURIComponent(user.nickname)}`;
}

// SHOP is the shop of gifts, "Подарки" of the left menu.
export const SHOP = '/giftshop';

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

// giftsBlock is openvk's block "Подарки" of a page: how many, "Все" and
// the newest. Sending one is "Отправить подарок" under the photo, buying
// one is the shop.
export function giftsBlock(user, page) {
  return [
    el('div', { className: 'section-title', textContent: tr('Подарки') }),
    el('div', { className: 'subtitle' }, el('span', { textContent: giftsCount(page.total) }),
      el('span', {}, el('a', { href: giftsPath(user), textContent: tr('Все') }))),
    el('a', { className: 'gift-row', href: giftsPath(user) }, ...page.gifts.map(giftThumb)),
  ];
}

// plainClick is a click that the page takes for itself: not one that
// opens the link in a new tab or window.
export function plainClick(event) {
  return event.button === 0 && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey;
}

// giftRow is a collection of "Выбрать подарок", as openvk's gift_cat_row:
// its name, "Показать больше" to all its gifts when more is given, and the
// gifts — the first limit of them — each with its price and what is left
// of its edition on pointing, a sold out one dimmed. Each leads to
// href(gift), its "Подтверждение"; a plain click calls pick with the gift
// and its price instead, as openvk's box.
export function giftRow(collection, { more = '', limit = Infinity, href }, pick) {
  return el('div', { className: 'gift-cat-row' },
    el('div', { className: 'gift-cat-info' },
      el('h4', { className: 'gift-cat-header', textContent: giftCollections[collection.id] ?? tr('Подарки') }),
      collection.prime ? el('a', { className: 'gift-cat-prime', href: '/prime', textContent: tr('Только для Prime') }) : null,
      more ? el('a', { className: 'button', href: more, textContent: tr('Показать больше') }) : null),
    el('div', { className: 'gift-grid' }, ...giftOffers(collection).slice(0, limit).map(({ gift, price, left }) => el('a', {
      className: `gift-sel${left === 0 ? ' disabled' : ''}${left !== Infinity ? ' has-limit' : ''}`,
      href: href(gift),
      title: giftNames[gift] ?? tr('Подарок'),
      onclick: (event) => {
        if (left === 0) {
          event.preventDefault();
          return;
        }
        if (!plainClick(event)) return;
        event.preventDefault();
        pick(gift, price);
      },
    },
    giftPicture(gift),
    el('strong', { className: price > 0 ? 'gift-price' : 'gift-price free', textContent: giftPrice(price) }),
    left !== Infinity ? el('strong', { className: 'gift-limit', textContent: giftsLeft(left) }) : null,
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

// rate is what a vote is worth, asked once; 0 when unknown.
let rate = null;

// friends are those whom the box offers as recipients: the friends of
// the one who sends, asked once.
let friends = null;

function knownFriends(me) {
  friends ??= api.friends(me.nickname, 'all', '', 50).then((page) => page.users, () => []);

  return friends;
}

// NICKNAME is a nickname typed in whole, as the box takes it for someone
// who is not among the friends.
const NICKNAME = /^@?([A-Za-z][A-Za-z0-9_]*)$/;

// giftDialog is openvk's box "Отправить подарок" (openGiftDialog of its
// gifts.js): the gift, what it costs for all the recipients and what one
// has, "Получатель:" — the people chosen, each with "×", and a field that
// offers one's friends or takes a nickname — "Сообщение к подарку:",
// "Отправить анонимно" and the button. The gift goes to each in turn;
// done is called with how many went and the votes spent. With shop the
// box is the shop's: it offers "Себе" first, and one's own gift is
// bought, not sent.
export function giftDialog({ me, gift, price, recipients = [], shop = false, done = () => {} }) {
  const chosen = [...recipients];
  const modal = openModal(shop ? tr('Покупка подарка') : tr('Отправить подарок'), { tag: 'form' });
  const tokens = el('span', { className: 'gift-tokens' });
  const input = el('input', { type: 'text', className: 'gift-recipient-input', autocomplete: 'off' });
  const choices = el('div', { className: 'gift-choices', hidden: true });
  const box = el('div', { className: 'gift-recipient' }, tokens, input, choices);
  const cost = el('b', {});
  const caption = el('textarea', { maxLength: 250, placeholder: tr('Прикрепить сообщение...') });
  const anonymous = el('input', { type: 'checkbox' });
  const error = el('div', { className: 'error', hidden: true });
  const send = button(shop ? tr('Купить') : tr('Отправить подарок'), null, { type: 'submit' });
  const balance = el('div', { className: 'gift-send-balance', textContent: tr('У Вас {0}', balanceAmount(me.votes ?? 0)) });

  rate ??= api.voteRate().then((r) => r.kopecks, () => 0);
  rate.then((kopecks) => {
    if (kopecks) balance.append(` (${voteRate(kopecks)})`);
  });

  function draw() {
    tokens.replaceChildren(...chosen.map((person) => el('span', { className: 'gift-token' },
      el('span', { textContent: person.id === me.id ? tr('Себе') : fullName(person) }),
      el('a', { className: 'gift-token-remove', title: tr('Убрать'), textContent: '×', onclick: () => remove(person) }))));
    input.placeholder = chosen.length ? tr('Добавить +') : tr('Введите имя друга или никнейм...');
    cost.textContent = giftTotal(price, Math.max(chosen.length, 1));
  }

  function add(person) {
    hideError(error);
    if (person.id === me.id && !shop) {
      showError(error, tr('Вы не можете отправить подарок самому себе'));
      return;
    }
    if (!chosen.some((p) => p.id === person.id)) chosen.push(person);
    input.value = '';
    draw();
    offer();
    input.focus();
  }

  function remove(person) {
    chosen.splice(chosen.indexOf(person), 1);
    draw();
    input.focus();
  }

  // offer shows the friends that match what is typed; "Себе" leads in
  // the shop, and a nickname typed in whole is offered when no friend is.
  async function offer() {
    const query = input.value.trim();
    const people = recipientChoices(await knownFriends(me), query, chosen);
    const items = people.map((person) => el('a', { className: 'gift-choice', onclick: () => add(person) },
      el('img', { src: avatarURL(person, 'small'), alt: '' }), el('span', { textContent: fullName(person) })));
    if (shop && !query && !chosen.some((p) => p.id === me.id)) {
      items.unshift(el('a', { className: 'gift-choice', onclick: () => add(me) },
        el('img', { src: avatarURL(me, 'small'), alt: '' }), el('span', { textContent: tr('Себе') })));
    }
    const nickname = NICKNAME.exec(query)?.[1];
    if (!people.length && nickname) {
      items.push(el('a', { className: 'gift-choice', onclick: () => pickNickname(nickname) },
        el('span', { textContent: tr('@{0} (выбрать)', nickname) })));
    }
    choices.replaceChildren(...items);
    choices.hidden = items.length === 0 || document.activeElement !== input;
  }

  async function pickNickname(nickname) {
    try {
      add(await api.profile(nickname));
    } catch (e) {
      showError(error, e.status === 404 ? tr('Такого человека нет.') : e);
    }
  }

  input.addEventListener('focus', offer);
  input.addEventListener('input', offer);
  input.addEventListener('blur', () => {
    choices.hidden = true;
  });
  // A choice is made with the field still in focus.
  choices.addEventListener('mousedown', (event) => event.preventDefault());
  input.addEventListener('keydown', (event) => {
    if (event.key === 'Backspace' && !input.value && chosen.length) remove(chosen[chosen.length - 1]);
    if (event.key === 'Enter') {
      event.preventDefault();
      choices.querySelector('.gift-choice')?.click();
    }
  });
  box.addEventListener('click', (event) => {
    if (event.target === box || event.target === tokens) input.focus();
  });

  modal.box.classList.add('gift-send');
  modal.body.append(
    el('div', { className: 'gift-send-top' },
      giftPicture(gift),
      el('div', { className: 'gift-send-name', textContent: giftNames[gift] ?? tr('Подарок') }),
      el('div', { className: 'gift-send-cost' }, tr('Стоимость: '), cost),
      balance),
    el('div', { className: 'gift-send-field' }, el('b', { textContent: tr('Получатель:') }), box),
    el('div', { className: 'gift-send-field' }, el('b', { textContent: tr('Сообщение к подарку:') }), caption),
    el('div', { className: 'gift-send-field' }, el('label', {}, anonymous, tr(' Отправить анонимно'))),
    error);
  modal.actions.append(send, button(tr('Отмена'), modal.close, { secondary: true }));
  draw();

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);

    if (!chosen.length) {
      showError(error, tr('Пожалуйста, выберите получателя подарка'));
      return;
    }
    if (price * chosen.length > (me.votes ?? 0)) {
      showError(error, tr('Недостаточно голосов для покупки.'));
      return;
    }

    send.disabled = true;
    const failed = [];
    let sent = 0;
    let self = false;
    for (const person of [...chosen]) {
      try {
        // One's own gift is told to nobody: there is no one to hide from.
        await api.sendGift(person.nickname, gift, caption.value.trim(), person.id !== me.id && anonymous.checked);
        sent += 1;
        self ||= person.id === me.id;
        chosen.splice(chosen.indexOf(person), 1);
      } catch (e) {
        const reason = e.message === 'not enough votes' ? tr('Недостаточно голосов для покупки.') : errorText(e);
        failed.push(`${person.id === me.id ? tr('Себе') : fullName(person)}: ${reason}`);
      }
    }
    me.votes = Math.max((me.votes ?? 0) - sent * price, 0);
    balance.firstChild.textContent = tr('У Вас {0}', balanceAmount(me.votes));
    if (sent) done(sent, sent * price);

    if (!failed.length) {
      modal.close();
      sentBox(me, sent, self);
      return;
    }

    // What was not sent stays chosen, to try again.
    draw();
    send.disabled = false;
    showError(error, [sent ? tr('Отправлено {0} из {1}.', sent, sent + failed.length) : '', ...failed].filter(Boolean).join('\n'));
  });

  if (!chosen.length) input.focus();
  else caption.focus();
}

// sentBox is openvk's "Успех": "Подарок отправлен"; a gift bought for
// oneself alone leads to one's gifts.
function sentBox(me, sent, self) {
  const modal = openModal(tr('Успех'));

  if (self && sent === 1) {
    modal.body.append(el('span', {}, tr('Подарок куплен. Он уже на вашей странице — '),
      el('a', { href: giftsPath(me), textContent: tr('Мои подарки') }), '.'));
  } else {
    modal.body.append(sent === 1 ? tr('Подарок отправлен') : tr('Подарки отправлены: {0}.', sent));
  }
  modal.actions.append(button('OK', modal.close));
}

// primeOnly says that a gift of Prime takes it.
export function primeOnly() {
  const modal = openModal('Prime');

  modal.body.append(el('span', {}, tr('Эти подарки могут отправлять только подписчики '),
    el('a', { href: '/prime', textContent: 'Prime' }), '.'));
  modal.actions.append(button('OK', modal.close));
}
