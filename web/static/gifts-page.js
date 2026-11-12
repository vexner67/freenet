// "Подарки" of a person, as openvk's: the gifts sent to the page, newest
// first, and "Отправить подарок" — the collections to choose from, five
// gifts of each, "Показать больше" to all the gifts of one, and the box
// that sends one (giftDialog). /gifts is yours, /gifts?u=<nickname>
// someone's, ?act=pick chooses a gift for them,
// ?act=menu&pack=<collection> in a collection and
// ?act=confirm&pack=<collection>&elid=<gift> is openvk's "Подтверждение":
// the gift sent without the box. Your own catalogue starts without a recipient.

import { api, currentUser } from './api.js';
import { collectibleName, errorText, formatPostDate, genitiveName, giftCollections, giftImage, giftNames, giftOffers, votesCount } from './labels.js';
import { collectibleCard, confirmPath, giftDialog, giftPath, giftPicture, giftRow, giftsPath, menuPath, pickPath, primeOnly } from './gifts.js';
import { $, avatarURL, el, fullName, hideError, pagePath, renderChrome, showError, verifiedMark } from './layout.js';
import { button, openModal } from './modal.js';
import { separated } from './posts.js';
import { tr } from './i18n.js';
import { recipientPicker } from './recipient.js';

const params = new URLSearchParams(location.search);
const nickname = params.get('u') ?? '';
const act = params.get('act');
// The collection chosen with ?act=menu and ?act=confirm, and the gift of
// the latter; picking is all three.
const collection = act === 'menu' || act === 'confirm' ? params.get('pack') ?? '' : '';
const confirming = act === 'confirm';
const elid = confirming ? params.get('elid') ?? '' : '';
const picking = act === 'pick' || act === 'menu' || confirming;

let cursor = '';

function renderHead(owner, me) {
  const mine = owner.id === me?.id;
  const target = mine ? null : owner;
  const link = (href, textContent) => el('a', { href, textContent });

  document.title = `${confirming ? tr('Отправить подарок') : picking ? tr('Выбрать подарок') : tr('Подарки')} | Freenet`;
  const name = giftCollections[collection] ?? tr('Подарки');
  if (!picking) $('title').replaceChildren(link(pagePath(owner), fullName(owner)), tr(' » Подарки'));
  else if (!collection) $('title').replaceChildren(link(pagePath(owner), fullName(owner)), ' » ', link(pickPath(target), tr('Выбрать подарок')), tr(' » Коллекции'));
  else if (!confirming) {
    $('title').replaceChildren(link(pagePath(owner), fullName(owner)), ' » ', link(pickPath(target), tr('Выбрать подарок')), ' » ',
      link(pickPath(target), tr('Коллекции')), ` » ${name}`);
  } else {
    $('title').replaceChildren(link(pagePath(owner), fullName(owner)), ' » ', link(pickPath(target), tr('Выбрать подарок')), ' » ',
      link(pickPath(target), tr('Коллекции')), ' » ', link(menuPath(target, collection), name), tr(' » Подтверждение'));
  }

  $('tab-list').textContent = mine ? tr('Мои подарки') : tr('Подарки');
  $('tab-list').href = giftsPath(target);
  $('tab-send').textContent = tr('Отправить подарок');
  $('tab-send').href = confirming ? menuPath(target, collection) : pickPath(target);
  $('tab-send').hidden = !me;
  $(picking ? 'tab-send' : 'tab-list').classList.add('active');
  $('tab-page').textContent = mine ? tr('Вернуться на мою страницу') : tr('Вернуться к странице {0}', genitiveName(owner));
  $('tab-page').href = pagePath(owner);
  $('tabs').hidden = false;
}

// giftNode is a gift as openvk shows it, laid out as a post: who sent it,
// or "Подарок" for one whose sender the viewer is not told, the picture
// with what was written, the date and, for the owner of the page, the
// gift in return and "Удалить".
function giftNode(gift, mine) {
  const { sender } = gift;
  const menu = [el('span', { className: 'post-date', textContent: formatPostDate(gift.sentAt) })];

  if (gift.number) menu.push(el('a', { href: giftPath(gift.id), textContent: mine ? tr('Продать или передать') : tr('Подробнее') }));
  else if (mine) menu.push(el('a', { textContent: tr('Улучшить'), onclick: () => upgrade(gift, node) }));
  if (mine && sender) menu.push(el('a', { href: pickPath(sender), textContent: tr('Отправить подарок в ответ') }));
  if (mine) menu.push(el('a', { textContent: tr('Удалить'), onclick: () => remove(gift, node) }));

  const node = el('div', { className: 'post gift' },
    sender
      ? el('a', { className: 'post-avatar', href: pagePath(sender) }, el('img', { src: avatarURL(sender, 'small'), alt: '' }))
      : el('span', { className: 'post-avatar' }, el('img', { src: '/static/avatar.svg', alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' }, ...[sender
        ? [el('a', { className: 'post-name', href: pagePath(sender), textContent: fullName(sender) }),
          verifiedMark(sender),
          // Told only to the two of them: others see "Подарок".
          gift.anonymous ? el('span', { className: 'hint', textContent: tr(' (анонимно)') }) : null]
        : el('b', { className: 'post-name', textContent: tr('Подарок') })].flat()),
      el('div', { className: 'user-gift' },
        gift.number ? el('a', { href: giftPath(gift.id) }, collectibleCard(gift)) : giftPicture(gift.gift),
        gift.number ? el('div', { className: 'collectible-caption' }, el('b', { textContent: collectibleName(gift) }), saleNote(gift)) : null,
        gift.caption ? el('div', { className: 'gift-comment', textContent: gift.caption }) : null),
      el('div', { className: 'post-menu' }, el('span', {}, ...separated(menu))),
    ),
  );

  return node;
}

// saleNote says that a collectible is on the market or at an auction.
function saleNote(gift) {
  if (gift.salePrice) return el('span', { className: 'hint', textContent: tr(' — продаётся за {0}', votesCount(gift.salePrice)) });
  if (gift.auctioned) return el('span', { className: 'hint', textContent: tr(' — на аукционе') });

  return null;
}

// catalog is asked once, when a gift is upgraded: what it costs.
let catalog = null;

// upgrade is "Улучшить", as Telegram's: for votes the gift gets a number
// and three traits rolled by rarity, shown when it is done.
async function upgrade(gift, node) {
  hideError($('error'));
  catalog ??= api.giftCatalog();

  let price;
  try {
    price = (await catalog).upgradePrice;
  } catch (e) {
    showError($('error'), e);
    return;
  }

  const modal = openModal(tr('Улучшить подарок'));
  const error = el('div', { className: 'error', hidden: true });
  const go = button(tr('Улучшить за {0}', votesCount(price)), async () => {
    hideError(error);
    go.disabled = true;

    try {
      const upgraded = await api.upgradeGift(gift.id);
      modal.close();
      node.replaceWith(giftNode({ ...gift, ...upgraded }, true));
      upgradedBox(upgraded);
    } catch (e) {
      showError(error, e.message === 'not enough votes' ? tr('Недостаточно голосов для улучшения.') : e);
      go.disabled = false;
    }
  });

  modal.body.append(
    el('div', { className: 'gift-send-top' }, giftPicture(gift.gift)),
    el('p', { textContent: tr('Подарок «{0}» станет коллекционным: получит свой номер и три особенности — модель, фон и узор, — которые выпадают случайно, редкие реже. Коллекционный подарок можно продать, выставить на аукцион или передать.', giftNames[gift.gift] ?? tr('Подарок')) }),
    error);
  modal.actions.append(go, button(tr('Отмена'), modal.close, { secondary: true }));
}

// upgradedBox shows what the upgrade rolled.
function upgradedBox(gift) {
  const modal = openModal(collectibleName(gift));

  modal.body.append(el('div', { className: 'collectible-done' }, collectibleCard(gift, 'large')),
    el('p', { className: 'collectible-done' }, el('a', { href: giftPath(gift.id), textContent: tr('Открыть страницу подарка') })));
  modal.actions.append(button('OK', modal.close));
}

async function remove(gift, node) {
  hideError($('error'));

  try {
    await api.deleteGift(gift.id);
    node.replaceWith(el('div', { className: 'post-deleted', textContent: tr('Подарок удалён.') }));
  } catch (e) {
    showError($('error'), e);
  }
}

async function loadMore(owner, mine) {
  hideError($('error'));

  try {
    const page = await api.gifts(owner.nickname, cursor);

    if (!cursor) $('empty').hidden = page.total > 0;
    $('gifts').append(...page.gifts.map((gift) => giftNode(gift, mine)));
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    showError($('error'), e);
  }
}

// showCatalog is "Выбрать подарок": the collections, five gifts of each,
// or all the gifts of the one chosen. A gift opens openvk's box with the
// owner of the page chosen, or empty on the sender's own catalogue.
async function showCatalog(owner, me) {
  try {
    const { collections } = await api.giftCatalog();
    // The gifts of Prime are sent only by those who have it: the
    // rest are told where to get it.
    const mine = owner.id === me.id;
    const target = mine ? null : owner;
    const pick = (c) => (gift, price) => (c.prime && !me.prime
      ? primeOnly()
      : giftDialog({ me, gift, price, recipients: mine ? [] : [owner], allowSelf: mine }));
    const href = (c) => (gift) => confirmPath(target, c.id, gift);
    const rows = collection
      ? collections.filter((c) => c.id === collection).map((c) => giftRow(c, { href: href(c) }, pick(c)))
      : collections.map((c) => giftRow(c, { more: menuPath(target, c.id), limit: 5, href: href(c) }, pick(c)));

    $('catalog').replaceChildren(...rows);
    $('catalog').hidden = false;
    $('gift-market-link').hidden = false;
    $('empty').hidden = rows.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

// refuse is openvk's "Не удалось подарить" with why.
function refuse(why) {
  showError($('error'), `${tr('Не удалось подарить')}: ${why}`);
}

// showConfirm is openvk's "Подтверждение": the gift, "Ваше сообщение",
// "Отправить" and "Анонимно", refused as openvk refuses it — a gift not
// of the collection, one sold out, one of Prime without it and one the
// votes do not pay for. Once sent, the page says so, as openvk's
// "Подарок отправлен" does on the page of the recipient.
async function showConfirm(owner, me) {
  const { collections } = await api.giftCatalog();
  const pack = collections.find((c) => c.id === collection);
  const offer = pack && giftOffers(pack).find((o) => o.gift === elid);
  const mine = owner.id === me.id;

  if (!offer) return refuse(tr('Не удалось подтвердить права на подарок.'));
  if (offer.left === 0) return refuse(tr('Тираж этого подарка распродан.'));
  if (pack.prime && !me.prime) return refuse(tr('Эти подарки могут отправлять только подписчики Prime.'));
  if (offer.price > (me.votes ?? 0)) return refuse(tr('Недостаточно голосов для покупки.'));

  const caption = el('textarea', { className: 'gift-confirm-text', maxLength: 250, placeholder: tr('Ваше сообщение') });
  const anonymous = el('input', { type: 'checkbox' });
  const submit = el('input', { type: 'submit', className: 'button', value: tr('Отправить') });
  const form = el('form', { className: 'gift-confirm-form' });
  const recipient = mine ? confirmRecipient(form, me, submit, anonymous) : () => Promise.resolve(owner);
  form.append(caption, el('div', { className: 'gift-confirm-actions' }, submit, el('label', {}, anonymous, tr(' Анонимно'))));

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError($('error'));
    submit.disabled = true;

    try {
      const person = await recipient();
      if (!person) {
        refuse(tr('Пожалуйста, выберите получателя подарка'));
        submit.disabled = false;
        return;
      }
      const self = person.id === me.id;
      await api.sendGift(person.nickname, elid, caption.value.trim(), !self && anonymous.checked);
      form.replaceWith(el('div', { className: 'notice' }, el('b', { textContent: tr('Подарок отправлен') }), el('br'),
        tr('Вы отправили подарок {0} за {1}. ', fullName(person), votesCount(offer.price)),
        el('a', { href: pagePath(person), textContent: self ? tr('Вернуться на мою страницу') : tr('Вернуться к странице {0}', genitiveName(person)) })));
    } catch (e) {
      refuse(e.message === 'not enough votes' ? tr('Недостаточно голосов для покупки.') : errorText(e));
      submit.disabled = false;
    }
  });

  $('catalog').replaceChildren(el('div', { className: 'gift-confirm' },
    el('img', { className: 'gift-confirm-pic', src: giftImage(elid), alt: giftNames[elid] ?? tr('Подарок') }), form));
  $('catalog').hidden = false;
}

function confirmRecipient(form, me, submit, anonymous) {
  const to = el('input', { className: 'text', required: true, autocomplete: 'off' });
  const people = el('div', { className: 'recipient-options', hidden: true });
  const shown = el('div', { hidden: true });
  const update = () => {
    const self = to.value.trim().toLowerCase() === me.nickname.toLowerCase();
    submit.value = self ? tr('Купить себе') : tr('Отправить');
    anonymous.disabled = self;
  };
  const picker = recipientPicker({ to, people, shown, me, picked: update });
  to.addEventListener('input', update);
  form.append(el('label', {}, tr('Получатель: '), to), ' ', button(tr('Себе'), () => {
    to.value = me.nickname;
    picker.lookUp();
    update();
  }, { type: 'button' }), people, shown);

  return async () => picker.found() ?? await picker.lookUp();
}

async function show(me) {
  try {
    const owner = nickname ? await api.profile(nickname) : me;
    const mine = owner.id === me?.id;

    renderHead(owner, me);

    // As openvk's: a page that takes no gifts from the viewer is not
    // offered any.
    if (picking && !mine && !owner.access?.sendGift) {
      refuse(tr('Пользователь ограничил круг лиц, которые могут присылать ему подарки.'));
      return;
    }
    if (confirming) {
      await showConfirm(owner, me);
      return;
    }
    if (picking) {
      await showCatalog(owner, me);
      return;
    }

    $('more').addEventListener('click', () => loadMore(owner, mine));
    await loadMore(owner, mine);
  } catch (e) {
    $('title').textContent = e.status === 404 ? tr('Страница не найдена') : tr('Подарки');
    showError($('error'), e);
  }
}

if (location.pathname === '/giftshop') {
  location.replace(pickPath(null));
} else {
  const me = await currentUser().catch(() => null);
  renderChrome(me);

  if (!me && (picking || !nickname)) {
    location.replace('/');
  } else if (!nickname && !me.profileCompleted) {
    $('title').textContent = tr('Мои подарки');
    showError($('error'), tr('Сначала заполните свою страницу.'));
  } else {
    await show(me);
  }
}
