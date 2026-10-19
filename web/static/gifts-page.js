// "Подарки" of a person, as openvk's: the gifts sent to the page, newest
// first, and "Отправить подарок" — the collections to choose from, five
// gifts of each, "Показать больше" to all the gifts of one, and the box
// that sends one. /gifts is yours, /gifts?u=<nickname> someone's,
// ?act=pick chooses a gift for them and ?act=menu&pack=<collection> in a
// collection. On one's own page it is "Купить подарок": one for oneself.

import { api, currentUser } from './api.js';
import { formatPostDate, genitiveName, giftCollections, giftPrice, voteRate, votesCount } from './labels.js';
import { giftPicture, giftRow, giftsPath, menuPath, pickPath } from './gifts.js';
import { $, avatarURL, el, fullName, hideError, pagePath, renderChrome, showError, verifiedMark } from './layout.js';
import { button, openModal } from './modal.js';
import { separated } from './posts.js';

const params = new URLSearchParams(location.search);
const nickname = params.get('u') ?? '';
const act = params.get('act');
// The collection chosen with ?act=menu; picking is both.
const collection = act === 'menu' ? params.get('pack') ?? '' : '';
const picking = act === 'pick' || act === 'menu';

let cursor = '';

function renderHead(owner, me) {
  const mine = owner.id === me?.id;
  const link = (href, textContent) => el('a', { href, textContent });

  document.title = `${picking ? 'Выбрать подарок' : 'Подарки'} | Freenet`;
  if (!picking) $('title').replaceChildren(link(pagePath(owner), fullName(owner)), ' » Подарки');
  else if (!collection) $('title').replaceChildren(link(pagePath(owner), fullName(owner)), ' » ', link(pickPath(owner), 'Выбрать подарок'), ' » Коллекции');
  else {
    $('title').replaceChildren(link(pagePath(owner), fullName(owner)), ' » ', link(pickPath(owner), 'Выбрать подарок'), ' » ',
      link(pickPath(owner), 'Коллекции'), ` » ${giftCollections[collection] ?? 'Подарки'}`);
  }

  $('tab-list').textContent = mine ? 'Мои подарки' : 'Подарки';
  $('tab-list').href = giftsPath(owner);
  // One's own page buys a gift for oneself.
  $('tab-send').textContent = mine ? 'Купить подарок' : 'Отправить подарок';
  $('tab-send').href = pickPath(owner);
  $('tab-send').hidden = !me;
  $(picking ? 'tab-send' : 'tab-list').classList.add('active');
  $('tab-page').textContent = mine ? 'К моей странице' : `К странице ${genitiveName(owner)}`;
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

  if (mine && sender) menu.push(el('a', { href: pickPath(sender), textContent: 'Отправить подарок в ответ' }));
  if (mine) menu.push(el('a', { textContent: 'Удалить', onclick: () => remove(gift, node) }));

  const node = el('div', { className: 'post gift' },
    sender
      ? el('a', { className: 'post-avatar', href: pagePath(sender) }, el('img', { src: avatarURL(sender, 'small'), alt: '' }))
      : el('span', { className: 'post-avatar' }, el('img', { src: '/static/club.svg', alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' }, ...[sender
        ? [el('a', { className: 'post-name', href: pagePath(sender), textContent: fullName(sender) }),
          verifiedMark(sender),
          // Told only to the two of them: others see "Подарок".
          gift.anonymous ? el('span', { className: 'hint', textContent: ' (анонимно)' }) : null]
        : el('b', { className: 'post-name', textContent: 'Подарок' })].flat()),
      el('div', { className: 'user-gift' },
        giftPicture(gift.gift),
        gift.caption ? el('div', { className: 'gift-comment', textContent: gift.caption }) : null),
      el('div', { className: 'post-menu' }, el('span', {}, ...separated(menu))),
    ),
  );

  return node;
}

async function remove(gift, node) {
  hideError($('error'));

  try {
    await api.deleteGift(gift.id);
    node.replaceWith(el('div', { className: 'post-deleted', textContent: 'Подарок удалён.' }));
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

// rate is what a vote is worth, asked with the page; 0 when unknown.
const rate = api.voteRate().then((r) => r.kopecks, () => 0);

// confirm is openvk's box "Отправить подарок": the gift, what it costs and
// what one has, to whom, "Сообщение к подарку" and "Отправить анонимно".
// One's own page buys the gift for oneself, and anonymous there is
// nobody.
function confirm(owner, me, gift, price) {
  const mine = owner.id === me.id;
  const modal = openModal(mine ? 'Купить подарок' : 'Отправить подарок', { tag: 'form' });
  const caption = el('textarea', { maxLength: 250, placeholder: 'Прикрепить сообщение...' });
  const anonymous = el('input', { type: 'checkbox' });
  const error = el('div', { className: 'error', hidden: true });
  const send = button(mine ? 'Купить подарок' : 'Отправить подарок', null, { type: 'submit' });
  const balance = el('div', { className: 'gift-send-balance', textContent: `У Вас ${votesCount(me.votes ?? 0)}` });

  rate.then((kopecks) => {
    if (kopecks) balance.append(` (${voteRate(kopecks)})`);
  });

  modal.box.classList.add('gift-send');
  modal.body.append(
    el('div', { className: 'gift-send-top' },
      giftPicture(gift),
      el('div', { className: 'gift-send-cost' }, 'Стоимость: ', el('b', { textContent: giftPrice(price) })),
      balance),
    el('div', { className: 'gift-send-field' },
      el('b', { textContent: 'Получатель:' }),
      el('div', { className: 'gift-recipient' }, el('span', { textContent: fullName(owner) }))),
    el('div', { className: 'gift-send-field' }, el('b', { textContent: 'Сообщение к подарку:' }), caption),
  );
  if (!mine) modal.body.append(el('div', { className: 'gift-send-field' }, el('label', {}, anonymous, ' Отправить анонимно')));
  modal.body.append(error);
  modal.actions.append(send, button('Отмена', modal.close, { secondary: true }));

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    send.disabled = true;

    try {
      await api.sendGift(owner.nickname, gift, caption.value.trim(), anonymous.checked);
      modal.close();
      me.votes = Math.max((me.votes ?? 0) - price, 0);
      sent(owner, mine);
    } catch (e) {
      showError(error, e.message === 'not enough votes' ? 'Недостаточно голосов для покупки.' : e);
      send.disabled = false;
    }
  });
  caption.focus();
}

// sent is openvk's "Подарок отправлен"; one bought for oneself leads to
// one's gifts.
function sent(owner, mine) {
  const modal = openModal('Успех');

  modal.body.append(mine
    ? el('span', {}, 'Подарок куплен. Он уже на вашей странице — ', el('a', { href: giftsPath(owner), textContent: 'Мои подарки' }), '.')
    : 'Подарок отправлен');
  modal.actions.append(button('OK', modal.close));
}

// showCatalog is "Выбрать подарок": the collections, five gifts of each,
// or all the gifts of the one chosen.
async function showCatalog(owner, me) {
  try {
    const { collections } = await api.giftCatalog();
    const pick = (gift, price) => confirm(owner, me, gift, price);
    const rows = collection
      ? collections.filter((c) => c.id === collection).map((c) => giftRow(c, {}, pick))
      : collections.map((c) => giftRow(c, { more: menuPath(owner, c.id), limit: 5 }, pick));

    $('catalog').replaceChildren(...rows);
    $('catalog').hidden = false;
    $('empty').hidden = rows.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

async function show(me) {
  try {
    const owner = nickname ? await api.profile(nickname) : me;
    const mine = owner.id === me?.id;

    renderHead(owner, me);

    if (picking) {
      await showCatalog(owner, me);
      return;
    }

    $('more').addEventListener('click', () => loadMore(owner, mine));
    await loadMore(owner, mine);
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Страница не найдена' : 'Подарки';
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me && (picking || !nickname)) {
  location.replace('/');
} else if (!nickname && !me.profileCompleted) {
  $('title').textContent = 'Мои подарки';
  showError($('error'), 'Сначала заполните свою страницу.');
} else {
  await show(me);
}
