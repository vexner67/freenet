// "Подарки" of a person, as openvk's: the gifts sent to the page, newest
// first, and "Отправить подарок" — the collections to choose from and the
// box that sends one. /gifts is yours, /gifts?u=<nickname> someone's,
// ?act=pick chooses a gift for them.

import { api, currentUser } from './api.js';
import { formatPostDate, genitiveName, giftCollections } from './labels.js';
import { giftPicture, giftsPath, pickPath } from './gifts.js';
import { $, avatarURL, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { separated } from './posts.js';

const params = new URLSearchParams(location.search);
const nickname = params.get('u') ?? '';
const picking = params.get('act') === 'pick';

let cursor = '';

function renderHead(owner, me) {
  const mine = owner.id === me?.id;
  const name = el('a', { href: pagePath(owner), textContent: fullName(owner) });

  document.title = `${picking ? 'Выбрать подарок' : 'Подарки'} | Freenet`;
  if (picking) $('title').replaceChildren(name, ' » Выбрать подарок » Коллекции');
  else $('title').replaceChildren(name, ' » Подарки');

  $('tab-list').textContent = mine ? 'Мои подарки' : 'Подарки';
  $('tab-list').href = giftsPath(owner);
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

// confirm is openvk's "Подтверждение": the gift, "Ваше сообщение",
// "Анонимно" and "Отправить".
function confirm(owner, gift) {
  const modal = openModal('Отправить подарок', { tag: 'form' });
  const caption = el('textarea', { maxLength: 250, placeholder: 'Ваше сообщение' });
  const anonymous = el('input', { type: 'checkbox' });
  const error = el('div', { className: 'error', hidden: true });
  const send = button('Отправить', null, { type: 'submit' });

  modal.box.classList.add('gift-send');
  modal.body.append(giftPicture(gift), caption, el('label', {}, anonymous, ' Анонимно'), error);
  modal.actions.append(send, button('Отмена', modal.close, { secondary: true }));

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    send.disabled = true;

    try {
      await api.sendGift(owner.nickname, gift, caption.value.trim(), anonymous.checked);
      modal.close();
      $('sent').hidden = false;
      scrollTo(0, 0);
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });
  caption.focus();
}

// showCatalog is "Выбрать подарок": the collections, each with its gifts.
async function showCatalog(owner) {
  try {
    const { collections } = await api.giftCatalog();

    $('catalog').replaceChildren(...collections.map((collection) => el('div', { className: 'gift-collection' },
      el('h4', { className: 'gift-collection-title', textContent: giftCollections[collection.id] ?? 'Подарки' }),
      el('div', { className: 'gift-grid' }, ...collection.gifts.map((gift) => el('a', { className: 'gift-sel', onclick: () => confirm(owner, gift) },
        giftPicture(gift),
        el('strong', { className: 'gift-price', textContent: 'Бесплатно' }),
      ))),
    )));
    $('catalog').hidden = false;
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
      await showCatalog(owner);
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
