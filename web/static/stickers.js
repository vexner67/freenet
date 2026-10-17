// Stickers in dialogs and chats, as openvk's: a sticker is a message by
// itself, shown as a picture, and "Стикеры" under the form opens the packs
// the user has added, with the way to the "Магазин стикеров".

import { api } from './api.js';
import { stickerImage, stickerPackName } from './labels.js';
import { el, showError } from './layout.js';

// stickerNode is the sticker a message is: its picture, or the word for
// one this site does not know yet.
export function stickerNode(sticker) {
  const word = () => el('div', { className: 'im-message-text hint', textContent: 'Стикер' });
  const src = stickerImage(sticker);
  if (!src) return word();

  // A sticker of a pack its author has deleted has no picture any more.
  const image = el('img', { className: 'sticker', src, alt: 'Стикер', title: 'Стикер', loading: 'lazy' });
  image.addEventListener('error', () => image.replaceWith(word()), { once: true });

  return image;
}

// initStickers makes link open the panel of stickers under the form, in
// panel; a sticker clicked there goes to onPick(sticker). The packs are
// read when the panel first opens.
export function initStickers({ link, panel, error, onPick }) {
  let loaded = false;

  const show = async () => {
    if (loaded) return;

    try {
      const { packs } = await api.stickers();
      const added = packs.filter((p) => p.added);

      loaded = true;
      panel.replaceChildren(
        ...(added.length ? added.map((pack) => el('div', { className: 'sticker-pack' },
          el('b', { className: 'sticker-pack-title', textContent: stickerPackName(pack) }),
          el('div', { className: 'sticker-grid' }, ...pack.stickers.map((sticker) => el('a', {
            className: 'sticker-pick',
            title: 'Отправить стикер',
            onclick: () => {
              panel.hidden = true;
              onPick(sticker);
            },
          }, stickerNode(sticker)))),
        )) : [el('p', { className: 'hint', textContent: 'У вас пока нет стикеров' })]),
        el('a', { className: 'sticker-store', href: '/stickers', textContent: 'Магазин стикеров' }),
      );
    } catch (e) {
      panel.hidden = true;
      showError(error, e);
    }
  };

  link.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    if (!panel.hidden) show();
  });
}
