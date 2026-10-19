// "Стикеры" (/stickers, also /shop), as openvk's: the "Магазин стикеров" —
// the packs as cards in the sections "Популярные", "Бесплатные" and "Все",
// with a search, each opening the box "Информация о наборе" that adds or
// buys it — with ?act=settings, "Установленные наборы" — the packs one has,
// each with "Убрать набор" — and with ?act=author, "Студия стикеров": the
// packs one made of one's photos, made (?act=new) and changed
// (?act=edit&pack=) by a form. A pack bought once comes back free; the
// votes paid for a pack of a person go to its author. ?pack=<id> opens the
// box of the pack, as the link it gives does. The gifts the shop sold once
// are chosen from a person's page now: ?act=gifts leads to one's own.

import { api, currentUser } from './api.js';
import { STICKER_SECTIONS, installsCount, shopPacks, stickerImage, stickerPackAction, stickerPackName, stickerPackPrice, stickersCount, votesCount } from './labels.js';
import { pickPath } from './gifts.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { stickerNode } from './stickers.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const act = ['settings', 'author', 'new', 'edit', 'gifts'].includes(params.get('act')) ? params.get('act') : 'shop';
const section = Object.hasOwn(STICKER_SECTIONS, params.get('section')) ? params.get('section') : 'popular';
const query = params.get('q') ?? '';

// As many stickers as a pack holds, and as many photos as one request
// adds to it (the gateway takes no more).
const MAX_PACK_STICKERS = 40;
const PHOTOS_PER_REQUEST = 10;

let me = null;
// packs are those of the catalogue the page shows, kept to redraw them.
let packs = [];

// thumb is the picture of a pack: its first sticker, as openvk's cover.
function thumb(pack) {
  const src = pack.stickers.length ? stickerImage(pack.stickers[0]) : null;
  if (!src) return el('div', { className: 'stickers-pack-thumb-empty' });

  return el('img', { src, alt: stickerPackName(pack), loading: 'lazy' });
}

// author is who made a pack, a link to their page; a pack of the site
// names nobody.
function author(pack) {
  return pack.author ? el('a', { href: pagePath(pack.author), textContent: fullName(pack.author) }) : null;
}

// packCard is a pack as a card of openvk's shop: the cover, the name, who
// made it, the start of what they said of it and, at the foot, what it
// costs or that it is one's own — with "Убрать набор" among one's packs.
// A click opens the box of the pack; in the studio, its form.
function packCard(pack) {
  const studio = act === 'author';
  const edit = `/stickers?act=edit&pack=${encodeURIComponent(pack.id)}`;
  const open = (event) => {
    event.preventDefault();
    openPack(pack);
  };
  const link = (props, ...children) => el('a', studio ? { ...props, href: edit } : { ...props, href: `/stickers?pack=${encodeURIComponent(pack.id)}`, onclick: open }, ...children);

  let foot;
  if (studio) {
    foot = [
      link({ className: 'stickers-pack-price paid', textContent: tr('Редактировать') }),
      el('div', { className: 'stickers-pack-installs', textContent: tr('Всего установок: {0}', pack.holders ?? 0) }),
    ];
  } else {
    const price = stickerPackPrice(pack);
    foot = [link({ className: `stickers-pack-price ${price.kind}`, textContent: price.text })];
    if (act === 'settings') foot.push(button(tr('Убрать набор'), () => confirmRemove(pack), { secondary: true }));
  }

  return el('div', { className: 'stickers-pack-card' },
    link({ className: 'stickers-pack-thumb' }, thumb(pack)),
    el('div', { className: 'stickers-pack-name' }, link({ textContent: stickerPackName(pack), title: stickerPackName(pack) })),
    studio
      ? el('div', { className: 'stickers-pack-author', textContent: pack.published ? tr('в магазине') : tr('не опубликован') })
      : pack.author ? el('div', { className: 'stickers-pack-author' }, author(pack)) : null,
    el('div', { className: 'stickers-pack-desc', textContent: pack.about ?? '' }),
    el('div', { className: 'stickers-pack-footer' }, ...foot),
  );
}

// openPack is openvk's box "Информация о наборе": the cover, the name, who
// made it and what they said, the button that adds or buys it, how many
// stickers and the link to the box, and the stickers.
function openPack(pack) {
  const modal = openModal(stickerPackName(pack));
  const error = el('div', { className: 'error', hidden: true });
  const action = button(stickerPackAction(pack), () => add(pack, action, error));
  const copy = el('a', { className: 'stickers-modal-copy', textContent: tr('Скопировать ссылку') });

  action.disabled = Boolean(pack.added);
  copy.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(`${location.origin}/stickers?pack=${encodeURIComponent(pack.id)}`);
      copy.textContent = tr('Ссылка скопирована!');
      setTimeout(() => { copy.textContent = tr('Скопировать ссылку'); }, 2000);
    } catch {
      // A browser that keeps the clipboard shut leaves the link as it was.
    }
  });

  // What one has to pay with, under the button of a pack one buys.
  const balance = me && pack.price && !pack.bought
    ? el('span', { className: 'hint', textContent: tr('У Вас {0}', votesCount(me.votes ?? 0)) })
    : null;

  modal.box.classList.add('stickers-pack-modal');
  modal.body.append(
    el('div', { className: 'stickers-modal-header' },
      el('div', { className: 'stickers-modal-cover' }, thumb(pack)),
      el('div', { className: 'stickers-modal-info' },
        el('div', { className: 'stickers-modal-title', textContent: stickerPackName(pack) }),
        pack.author ? el('div', { className: 'stickers-modal-author' }, author(pack)) : null,
        pack.about ? el('div', { className: 'stickers-modal-desc', textContent: pack.about }) : null,
        el('div', { className: 'stickers-modal-actions' }, action, balance),
      ),
    ),
    error,
    el('div', { className: 'stickers-modal-subbar' },
      el('span', { className: 'stickers-modal-count', textContent: stickersCount(pack.stickers.length) }),
      copy),
    el('div', { className: 'stickers-modal-grid' },
      ...pack.stickers.map((sticker) => el('div', { className: 'stickers-modal-item' }, stickerNode(sticker)))),
  );
  modal.actions.append(button(tr('Закрыть'), modal.close));
}

// add takes the pack; a guest is sent to sign in. Taken, the box says
// "Приобретён" and the cards follow.
async function add(pack, action, error) {
  if (!me) {
    location.assign('/');
    return;
  }

  hideError(error);
  action.disabled = true;

  try {
    await api.addStickerPack(pack.id);
    if (pack.price && !pack.bought) me.votes = Math.max((me.votes ?? 0) - pack.price, 0);
    pack.bought = true;
    pack.added = true;
    action.textContent = stickerPackAction(pack);
    action.nextElementSibling?.remove();
    render();
  } catch (e) {
    action.disabled = false;
    showError(error, e.message === 'not enough votes' ? tr('Недостаточно голосов для покупки.') : e);
  }
}

// confirmRemove is openvk's "Вы уверены, что хотите удалить этот набор из
// своих стикеров?".
function confirmRemove(pack) {
  const modal = openModal(tr('Предупреждение'));

  modal.body.append(tr('Вы уверены, что хотите удалить этот набор из своих стикеров?'));
  modal.actions.append(button(tr('Да'), async () => {
    modal.close();
    hideError($('error'));

    try {
      await api.removeStickerPack(pack.id);
      pack.added = false;
      render();
    } catch (e) {
      showError($('error'), e);
    }
  }), button(tr('Отмена'), modal.close, { secondary: true }));
}

// render draws the cards of the packs the page shows.
function render() {
  const shown = act === 'settings' ? packs.filter((p) => p.added)
    : act === 'author' ? packs
      : shopPacks(packs.filter((p) => p.published || p.added), section, query);

  $('packs').replaceChildren(...shown.map(packCard));
  $('empty').hidden = shown.length > 0;
}

async function showShop() {
  $('sections').hidden = false;
  $(`section-${section}`).classList.add('active');
  $('search-section').value = section;
  $('search-query').value = query;

  try {
    ({ packs } = await api.stickers());
    render();

    const asked = packs.find((p) => p.id === params.get('pack'));
    if (asked) openPack(asked);
  } catch (e) {
    showError($('error'), e);
  }
}

async function showSettings() {
  $('summary-text').textContent = tr('Установленные наборы');
  $('summary').hidden = false;

  try {
    ({ packs } = await api.stickers());
    render();
  } catch (e) {
    showError($('error'), e);
  }
}

async function showStudio() {
  $('summary-text').textContent = tr('Автор стикеров');
  $('create').hidden = false;
  $('summary').hidden = false;

  try {
    ({ packs } = await api.myStickerPacks());
    render();
  } catch (e) {
    showError($('error'), e);
  }
}

// photoOf is the photo a sticker of a person's pack is, its ID as the
// gateway takes it.
function photoOf(sticker) {
  return `${sticker.slice(0, 8)}-${sticker.slice(8, 12)}-${sticker.slice(12, 16)}-${sticker.slice(16, 20)}-${sticker.slice(20)}`;
}

// initForm is the form of a pack, new or the one given: the stickers it
// has, those one uploads and takes out; "Сохранить набор" adds the new
// photos ten at a time, as the gateway takes them.
function initForm(pack) {
  // items are the stickers in their order: { sticker } of the pack, or
  // { photo } uploaded (undefined while uploading).
  let items = (pack?.stickers ?? []).map((sticker) => ({ sticker }));
  const removed = [];
  let id = pack?.id;
  let uploading = 0;

  $('studio').hidden = false;
  $('studio-title').textContent = pack ? tr('Редактирование набора') : tr('Новый набор стикеров');
  $('pack-save').textContent = pack ? tr('Сохранить') : tr('Сохранить набор');
  $('pack-delete-line').hidden = !pack;

  if (pack) {
    $('pack-name').value = pack.name ?? '';
    $('pack-about').value = pack.about ?? '';
    $('pack-price').value = String(pack.price ?? 0);
    $('pack-published').checked = Boolean(pack.published);
  }

  const render = () => {
    $('pack-stickers').replaceChildren(...items.map((item) => el('div', { className: 'sticker-studio-item' },
      item.sticker ? stickerNode(item.sticker)
        : item.photo ? el('img', { src: item.photo.small, alt: tr('Стикер') })
          : el('div', { className: 'attachment-loading', textContent: tr('Загрузка…') }),
      el('a', {
        className: 'profile-link',
        textContent: tr('Удалить'),
        onclick: () => {
          if (item.sticker) removed.push(photoOf(item.sticker));
          else if (item.saved) removed.push(item.photo.id);
          items = items.filter((i) => i !== item);
          render();
        },
      }),
    )));
    $('pack-add').hidden = items.length >= MAX_PACK_STICKERS;
  };

  // upload sends the files one after another, so the pack keeps the order
  // they were picked in.
  const upload = async (files) => {
    hideError($('error'));

    const room = MAX_PACK_STICKERS - items.length;
    if (files.length > room) showError($('error'), tr('В наборе может быть не больше {0} стикеров.', MAX_PACK_STICKERS));

    const picked = files.slice(0, Math.max(room, 0)).map((file) => ({ file }));
    items.push(...picked);
    uploading++;
    render();

    for (const item of picked) {
      try {
        item.photo = await api.uploadPhoto(item.file);
      } catch (e) {
        items = items.filter((i) => i !== item);
        showError($('error'), e);
      }
      render();
    }

    uploading--;
  };

  $('pack-add').addEventListener('click', () => $('pack-files').click());
  $('pack-files').addEventListener('change', () => {
    const files = [...$('pack-files').files];
    $('pack-files').value = '';
    if (files.length) upload(files);
  });

  $('studio-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError($('error'));

    if (uploading) {
      showError($('error'), tr('Дождитесь, пока загрузятся все стикеры.'));
      return;
    }

    const draft = {
      name: $('pack-name').value,
      about: $('pack-about').value,
      price: Number($('pack-price').value) || 0,
      published: $('pack-published').checked,
    };
    $('pack-save').disabled = true;

    // The new photos go ten a request. A save that failed partway is
    // taken up again by the next "Сохранить": the pack is made already,
    // and the photos it holds are kept as its stickers.
    try {
      let fresh = items.filter((i) => i.photo && !i.saved);
      // A pack made just now says all it says; one there was has its
      // words saved by an edit, even with no new photos.
      let said = false;

      if (!id) {
        const first = fresh.slice(0, PHOTOS_PER_REQUEST);
        ({ id } = await api.createStickerPack({ ...draft, photos: first.map((i) => i.photo.id) }));
        first.forEach((i) => { i.saved = true; });
        fresh = fresh.slice(PHOTOS_PER_REQUEST);
        said = true;
      }

      while (fresh.length || !said) {
        const chunk = fresh.slice(0, PHOTOS_PER_REQUEST);
        await api.editStickerPack(id, { ...draft, addPhotos: chunk.map((i) => i.photo.id), removePhotos: removed });
        chunk.forEach((i) => { i.saved = true; });
        removed.length = 0;
        fresh = fresh.slice(PHOTOS_PER_REQUEST);
        said = true;
      }

      location.assign('/stickers?act=author');
    } catch (e) {
      $('pack-save').disabled = false;
      showError($('error'), e);
    }
  });

  $('pack-delete').addEventListener('click', () => confirmDelete(pack));
  render();
}

// confirmDelete deletes a pack of one's own with its photos; a pack
// others have taken stays.
function confirmDelete(pack) {
  const modal = openModal(tr('Удалить набор'));

  modal.body.append(tr('Вы уверены, что хотите удалить набор «{0}»? Его стикеры пропадут из переписки.', stickerPackName(pack)));
  modal.actions.append(button(tr('Удалить'), async () => {
    modal.close();
    hideError($('error'));

    try {
      await api.deleteStickerPack(pack.id);
      location.assign('/stickers?act=author');
    } catch (e) {
      showError($('error'), e);
    }
  }), button(tr('Отмена'), modal.close, { secondary: true }));
}

async function showEdit() {
  try {
    const { packs } = await api.myStickerPacks();
    const pack = packs.find((p) => p.id === params.get('pack'));

    if (!pack) {
      showError($('error'), tr('Набор стикеров не найден.'));
      return;
    }

    initForm(pack);
  } catch (e) {
    showError($('error'), e);
  }
}

const titles = {
  shop: tr('Стикеры'),
  settings: tr('Установленные наборы'),
  author: tr('Студия стикеров'),
  new: tr('Новый набор стикеров'),
  edit: tr('Редактирование набора'),
};

me = await currentUser().catch(() => null);

if (act === 'gifts') {
  // The gifts one bought here are bought from one's own page now.
  location.replace(me?.nickname ? pickPath(me) : '/');
} else if (act !== 'shop' && !me) {
  location.replace('/');
} else {
  renderChrome(me);
  document.title = `${titles[act]} | Freenet`;
  $(['shop', 'settings'].includes(act) ? `tab-${act}` : 'tab-author').classList.add('active');
  $('tab-settings').hidden = !me;
  $('tab-author').hidden = !me;
  $('tab-dialogs').hidden = !me;

  if (act === 'settings') await showSettings();
  else if (act === 'author') await showStudio();
  else if (act === 'new') initForm(null);
  else if (act === 'edit') await showEdit();
  else await showShop();
}
