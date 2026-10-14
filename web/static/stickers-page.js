// Stickers, as openvk's /stickers: "Магазин стикеров" — every pack with
// its stickers, what it costs and the button that adds it — and, with
// ?act=settings, "Установленные наборы" — the packs one has, each with
// "Убрать набор". A pack bought once comes back free.

import { api, currentUser } from './api.js';
import { stickerPackAction, stickerPacks, unusedVotes, votesCount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { stickerNode } from './stickers.js';

const settings = new URLSearchParams(location.search).get('act') === 'settings';

let me = null;

// showBalance is "У Вас N неиспользованных голосов.", as over the gifts.
function showBalance() {
  const [before, votes, after] = unusedVotes(me.votes ?? 0);

  $('balance').replaceChildren(before, el('b', { textContent: votes }), after, ' ',
    el('a', { href: '/settings?act=finance.top-up', textContent: 'Есть ваучер?' }));
  $('balance').hidden = false;
}

// packNode is a pack as a card of openvk's shop: its name, its stickers,
// what it costs and its button — or "Убрать набор" among one's own.
function packNode(pack) {
  const action = settings
    ? button('Убрать набор', () => confirmRemove(pack), { secondary: true })
    : stickerButton(pack);

  return el('div', { className: 'sticker-pack-card' },
    el('div', { className: 'sticker-pack-head' },
      el('b', { textContent: stickerPacks[pack.id] ?? 'Стикеры' }),
      el('span', { className: 'hint', textContent: pack.price ? votesCount(pack.price) : 'Бесплатно' }),
    ),
    el('div', { className: 'sticker-grid' }, ...pack.stickers.map(stickerNode)),
    el('div', { className: 'sticker-pack-actions' }, action),
  );
}

// stickerButton adds the pack; a guest is sent to sign in, and an added
// pack says so.
function stickerButton(pack) {
  const text = stickerPackAction(pack);
  if (pack.added) return el('span', { className: 'hint', textContent: text });
  if (!me) return button(text, () => location.assign('/'));

  const add = button(text, async () => {
    hideError($('error'));
    add.disabled = true;

    try {
      await api.addStickerPack(pack.id);
      if (pack.price && !pack.bought) me.votes = Math.max((me.votes ?? 0) - pack.price, 0);
      $('added').hidden = false;
      await show();
      scrollTo(0, 0);
    } catch (e) {
      add.disabled = false;
      showError($('error'), e.message === 'not enough votes' ? 'Недостаточно голосов для покупки.' : e);
    }
  });

  return add;
}

// confirmRemove is openvk's "Вы уверены, что хотите удалить этот набор из
// своих стикеров?".
function confirmRemove(pack) {
  const modal = openModal('Убрать набор');

  modal.body.append('Вы уверены, что хотите удалить этот набор из своих стикеров?');
  modal.actions.append(button('Убрать', async () => {
    modal.close();
    hideError($('error'));

    try {
      await api.removeStickerPack(pack.id);
      await show();
    } catch (e) {
      showError($('error'), e);
    }
  }), button('Отмена', modal.close, { secondary: true }));
}

async function show() {
  try {
    const { packs } = await api.stickers();
    const shown = settings ? packs.filter((p) => p.added) : packs;

    if (me && !settings) showBalance();
    $('packs').replaceChildren(...shown.map(packNode));
    $('empty').textContent = settings ? 'У вас пока нет стикеров' : 'Наборов стикеров пока нет';
    $('empty').hidden = shown.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

me = await currentUser().catch(() => null);
renderChrome(me);

if (settings && !me) {
  location.replace('/');
} else {
  document.title = `${settings ? 'Установленные наборы' : 'Магазин стикеров'} | Freenet`;
  $(settings ? 'tab-settings' : 'tab-shop').classList.add('active');
  $('tab-settings').hidden = !me;
  $('tab-dialogs').hidden = !me;
  await show();
}
