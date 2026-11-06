// Stickers in dialogs and chats, as openvk's: a sticker is a message by
// itself, shown as a picture, and the tab "Стикеры" of the emoji picker opens
// the packs the user has added, with the way to the "Магазин стикеров".

import { api } from './api.js';
import { stickerImage, stickerPackName } from './labels.js';
import { el, showError } from './layout.js';
import { tr } from './i18n.js';

// stickerNode is the sticker a message is: its picture, or the word for
// one this site does not know yet. An eager one loads at once, even out
// of sight: the panel's are there before it opens.
export function stickerNode(sticker, { eager = false } = {}) {
  const word = () => el('div', { className: 'im-message-text hint', textContent: tr('Стикер') });
  const src = stickerImage(sticker);
  if (!src) return word();

  // A sticker of a pack its author has deleted has no picture any more.
  const image = el('img', { className: 'sticker', src, alt: tr('Стикер'), title: tr('Стикер'), loading: eager ? 'eager' : 'lazy' });
  image.addEventListener('error', () => image.replaceWith(word()), { once: true });

  return image;
}

// initStickers draws the stickers the user has added in panel, the tab
// "Стикеры" of the emoji picker (emoji.js); a sticker clicked there is
// sent at once by onPick(sticker), which tells whether it was, and then
// onSent closes the picker. Until then the panel stays, dimmed and deaf to
// more clicks, so that a slow server does not look as if nothing
// happened. The packs are read and drawn soon after the page, out of
// sight, so that the tab opens at once, with its pictures loaded. It
// returns show, which the tab calls.
export function initStickers({ panel, error, onPick, onSent }) {
  let loading = null;
  let sending = false;

  const pick = async (sticker) => {
    if (sending) return;

    sending = true;
    panel.classList.add('sending');
    try {
      if (await onPick(sticker)) onSent();
    } finally {
      sending = false;
      panel.classList.remove('sending');
    }
  };

  const load = async () => {
    try {
      const { packs } = await api.stickers();
      const added = packs.filter((p) => p.added);

      panel.replaceChildren(
        ...(added.length ? added.map((pack) => el('div', { className: 'sticker-pack' },
          el('div', { className: 'group-title' }, el('b', { textContent: stickerPackName(pack) })),
          el('div', { className: 'sticker-grid' }, ...pack.stickers.map((sticker) => el('a', {
            className: 'sticker-pick',
            title: tr('Отправить стикер'),
            onclick: () => pick(sticker),
          }, stickerNode(sticker, { eager: true })))),
        )) : [el('p', { className: 'hint', textContent: tr('У вас пока нет стикеров') })]),
        el('a', { className: 'sticker-store', href: '/stickers', textContent: tr('Магазин стикеров') }),
      );
    } catch (e) {
      loading = null;
      if (!panel.hidden) showError(error, e);
    }
  };
  const show = () => {
    loading ??= load();
  };

  setTimeout(show, 1500);
  return show;
}
