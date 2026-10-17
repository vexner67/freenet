// Stickers, as openvk's /stickers: "Магазин стикеров" — every pack with
// its stickers, what it costs and the button that adds it — with
// ?act=settings, "Установленные наборы" — the packs one has, each with
// "Убрать набор" — and with ?act=author, "Студия стикеров": the packs one
// made of one's photos, made (?act=new) and changed (?act=edit&pack=) by a
// form. A pack bought once comes back free; the votes paid for a pack of a
// person go to its author.

import { api, currentUser } from './api.js';
import { stickerPackAction, stickerPackName, unusedVotes, votesCount } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { stickerNode } from './stickers.js';

const params = new URLSearchParams(location.search);
const act = ['settings', 'author', 'new', 'edit'].includes(params.get('act')) ? params.get('act') : 'shop';
const settings = act === 'settings';

// As many stickers as a pack holds, and as many photos as one request
// adds to it (the gateway takes no more).
const MAX_PACK_STICKERS = 40;
const PHOTOS_PER_REQUEST = 10;

let me = null;

// showBalance is "У Вас N неиспользованных голосов.", as over the gifts.
function showBalance() {
  const [before, votes, after] = unusedVotes(me.votes ?? 0);

  $('balance').replaceChildren(before, el('b', { textContent: votes }), after, ' ',
    el('a', { href: '/settings?act=finance.top-up', textContent: 'Есть ваучер?' }));
  $('balance').hidden = false;
}

// packHead is what a pack is called and costs, then who made it and what
// they said of it.
function packHead(pack, ...more) {
  return [
    el('div', { className: 'sticker-pack-head' },
      el('b', { textContent: stickerPackName(pack) }),
      el('span', { className: 'hint', textContent: pack.price ? votesCount(pack.price) : 'Бесплатно' }),
      ...more,
    ),
    pack.author ? el('div', { className: 'sticker-pack-author hint' },
      'Автор: ', el('a', { href: pagePath(pack.author), textContent: fullName(pack.author) })) : '',
    pack.about ? el('div', { className: 'sticker-pack-about', textContent: pack.about }) : '',
  ];
}

// packNode is a pack as a card of openvk's shop: its name, its stickers,
// what it costs and its button — or "Убрать набор" among one's own.
function packNode(pack) {
  const action = settings
    ? button('Убрать набор', () => confirmRemove(pack), { secondary: true })
    : stickerButton(pack);

  return el('div', { className: 'sticker-pack-card' },
    ...packHead(pack),
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
    const shown = settings ? packs.filter((p) => p.added) : packs.filter((p) => p.published || p.added);

    if (me && !settings) showBalance();
    $('packs').replaceChildren(...shown.map(packNode));
    $('empty').textContent = settings ? 'У вас пока нет стикеров' : 'Наборов стикеров пока нет';
    $('empty').hidden = shown.length > 0;
  } catch (e) {
    showError($('error'), e);
  }
}

// authoredNode is a pack in the "Студия стикеров": whether the shop shows
// it, its stickers, how many have taken it and "Редактировать".
function authoredNode(pack) {
  return el('div', { className: 'sticker-pack-card' },
    ...packHead({ ...pack, author: null }, el('span', { className: 'hint', textContent: pack.published ? '• в магазине' : '• не опубликован' })),
    el('div', { className: 'sticker-grid' }, ...pack.stickers.map(stickerNode)),
    el('div', { className: 'sticker-pack-actions' },
      el('span', { className: 'hint', textContent: `Всего установок: ${pack.holders ?? 0}` }), ' | ',
      el('a', { href: `/stickers?act=edit&pack=${encodeURIComponent(pack.id)}`, textContent: 'Редактировать' })),
  );
}

async function showStudio() {
  $('studio-bar').hidden = false;

  try {
    const { packs } = await api.myStickerPacks();

    $('studio-count').textContent = packs.length ? `Ваши наборы стикеров: ${packs.length}` : 'Студия стикеров';
    $('packs').replaceChildren(...packs.map(authoredNode));
    $('empty').textContent = 'Вы ещё не создали ни одного набора. Наберите стикеры из своих фотографий и опубликуйте их в магазине — бесплатно или за голоса.';
    $('empty').hidden = packs.length > 0;
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
  $('studio-title').textContent = pack ? 'Редактирование набора' : 'Новый набор стикеров';
  $('pack-save').textContent = pack ? 'Сохранить' : 'Сохранить набор';
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
        : item.photo ? el('img', { src: item.photo.small, alt: 'Стикер' })
          : el('div', { className: 'attachment-loading', textContent: 'Загрузка…' }),
      el('a', {
        className: 'profile-link',
        textContent: 'Удалить',
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
    if (files.length > room) showError($('error'), `В наборе может быть не больше ${MAX_PACK_STICKERS} стикеров.`);

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
      showError($('error'), 'Дождитесь, пока загрузятся все стикеры.');
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
  const modal = openModal('Удалить набор');

  modal.body.append(`Вы уверены, что хотите удалить набор «${stickerPackName(pack)}»? Его стикеры пропадут из переписки.`);
  modal.actions.append(button('Удалить', async () => {
    modal.close();
    hideError($('error'));

    try {
      await api.deleteStickerPack(pack.id);
      location.assign('/stickers?act=author');
    } catch (e) {
      showError($('error'), e);
    }
  }), button('Отмена', modal.close, { secondary: true }));
}

async function showEdit() {
  try {
    const { packs } = await api.myStickerPacks();
    const pack = packs.find((p) => p.id === params.get('pack'));

    if (!pack) {
      showError($('error'), 'Набор стикеров не найден.');
      return;
    }

    initForm(pack);
  } catch (e) {
    showError($('error'), e);
  }
}

const titles = {
  shop: 'Магазин стикеров',
  settings: 'Установленные наборы',
  author: 'Студия стикеров',
  new: 'Новый набор стикеров',
  edit: 'Редактирование набора',
};

me = await currentUser().catch(() => null);
renderChrome(me);

if (act !== 'shop' && !me) {
  location.replace('/');
} else {
  document.title = `${titles[act]} | Freenet`;
  $(act === 'shop' || act === 'settings' ? `tab-${act}` : 'tab-author').classList.add('active');
  $('tab-settings').hidden = !me;
  $('tab-author').hidden = !me;
  $('tab-dialogs').hidden = !me;

  if (act === 'author') await showStudio();
  else if (act === 'new') initForm(null);
  else if (act === 'edit') await showEdit();
  else await show();
}
