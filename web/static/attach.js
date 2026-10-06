// Photos attached to a post being written, as on openvk: "Прикрепить
// фотографию" opens the file picker, each photo uploads at once and shows
// as a thumbnail with a cross to take it off.

import { api } from './api.js';
import { el, showError, hideError } from './layout.js';

export const MAX_PHOTOS = 10;
const MAX_BYTES = 8 << 20;

// initAttachments wires the link, the hidden file input and the list of
// thumbnails; onChange runs whenever photos come or go.
export function initAttachments({ link, input, list, error, onChange }) {
  // Entries keep the order the photos were picked in; an entry has its
  // photo once the upload is done.
  let entries = [];

  const uploaded = () => entries.filter((e) => e.photo);

  function render() {
    list.replaceChildren(...entries.map((entry) => el('div', { className: 'attachment' },
      entry.photo
        ? el('img', { src: entry.photo.small, alt: '' })
        : el('div', { className: 'attachment-loading', textContent: 'Загрузка…' }),
      entry.photo && el('a', { className: 'attachment-remove', title: 'Не прикреплять', textContent: '×', onclick: () => remove(entry) }),
    )));
    list.hidden = entries.length === 0;
    link.hidden = entries.length >= MAX_PHOTOS;
    onChange();
  }

  function remove(entry) {
    entries = entries.filter((e) => e !== entry);
    render();
  }

  async function upload(entry, file) {
    try {
      entry.photo = await api.uploadPhoto(file);
    } catch (e) {
      entries = entries.filter((x) => x !== entry);
      showError(error, e);
    }
    render();
  }

  link.addEventListener('click', () => input.click());
  input.addEventListener('change', () => {
    hideError(error);

    const files = [...input.files];
    input.value = '';

    const room = MAX_PHOTOS - entries.length;
    if (files.length > room) showError(error, `К записи можно прикрепить не больше ${MAX_PHOTOS} фотографий.`);

    for (const file of files.slice(0, room)) {
      // Checked here too, so a huge file is not sent for nothing.
      if (file.size > MAX_BYTES) {
        showError(error, 'Фотография больше 8 МБ.');
        continue;
      }

      const entry = { photo: null };
      entries.push(entry);
      upload(entry, file);
    }

    render();
  });

  return {
    ids: () => uploaded().map((e) => e.photo.id),
    count: () => entries.length,
    busy: () => entries.some((e) => !e.photo),
    clear() {
      entries = [];
      render();
    },
  };
}
