// Stickers in dialogs and chats, as openvk's: a sticker is a message by
// itself, shown as a picture, and "Стикеры" under the form opens the packs
// the user has added, with the way to the "Магазин стикеров".

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

// initStickers makes link open the panel of stickers under the form, in
// panel; a sticker clicked there is sent at once by onPick(sticker), which
// tells whether it was. Until then the panel stays, dimmed and deaf to
// more clicks, so that a slow server does not look as if nothing
// happened; sent, the panel closes. The packs are read and drawn soon
// after the page, out of sight, so that the panel opens at once, with
// its pictures loaded.
export function initStickers({ link, panel, error, onPick }) {
  let loading = null;
  let sending = false;

  const pick = async (sticker) => {
    if (sending) return;

    sending = true;
    panel.classList.add('sending');
    try {
      if (await onPick(sticker)) panel.hidden = true;
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
          el('b', { className: 'sticker-pack-title', textContent: stickerPackName(pack) }),
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
      panel.hidden = true;
    }
  };
  const show = () => {
    loading ??= load();
  };

  link.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    if (!panel.hidden) show();
  });
  setTimeout(show, 1500);
}
