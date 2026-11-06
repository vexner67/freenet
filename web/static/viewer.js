// The photo viewer, as openvk's PhotoViewer (photoViewerTemplate of
// messagebox.js, dialog.css): the page dimmed, a white box over it with
// "Фотография N из M" and "Закрыть" at its top, the photo under them —
// its left third shows the one before, its right third the one after —
// and the white cross at the corner of the screen. A click beside the box
// closes it; Esc closes, ← and → page. details(photo), if given, is shown
// under the photo: for a photo of an album its description, heart and
// comments.

import { adult, el, revealAdult } from './layout.js';
import { tr } from './i18n.js';

export function openViewer(photos, start, { details } = {}) {
  let index = start;

  const counter = el('b', { className: 'viewer-title' });
  const img = el('img', { className: 'viewer-photo', alt: '' });
  const prev = el('a', { className: 'viewer-nav viewer-prev', title: tr('Предыдущая') });
  const next = el('a', { className: 'viewer-nav viewer-next', title: tr('Следующая') });
  const close = el('a', { className: 'viewer-close', textContent: tr('Закрыть') });

  const footer = el('div', { className: 'viewer-details', hidden: !details });
  const box = el('div', { className: 'viewer-box' },
    el('div', { className: 'viewer-header' }, counter, el('div', {}, close)),
    el('div', { className: 'viewer-body' }, prev, next, img),
    footer,
  );
  // The bands at the sides of the screen close the viewer; the right one
  // carries the cross.
  const overlay = el('div', { className: 'viewer' },
    el('div', { className: 'viewer-overlay viewer-overlay-left' }),
    el('div', { className: 'viewer-overlay viewer-overlay-right' }, el('div', { className: 'viewer-close-icon' })),
    box);

  function show(i) {
    index = (i + photos.length) % photos.length;
    img.src = photos[index].large;
    adult(img, photos[index].nsfw);
    counter.textContent = photos.length > 1 ? tr('Фотография {0} из {1}', index + 1, photos.length) : tr('Фотография');
    if (details) footer.replaceChildren(details(photos[index]));
  }

  function onKey(event) {
    if (event.key === 'Escape') hide();
    if (event.key === 'ArrowLeft' && photos.length > 1) show(index - 1);
    if (event.key === 'ArrowRight' && photos.length > 1) show(index + 1);
  }

  function hide() {
    overlay.remove();
    document.body.classList.remove('viewer-open');
    document.removeEventListener('keydown', onKey);
  }

  prev.hidden = next.hidden = photos.length < 2;
  prev.addEventListener('click', () => show(index - 1));
  next.addEventListener('click', () => show(index + 1));
  // A click on a blurred photo shows the photo itself; on a single one it
  // closes the viewer, as the thirds of openvk do nothing there.
  img.addEventListener('click', () => {
    if (revealAdult(img)) return;

    if (photos.length < 2) hide();
  });
  close.addEventListener('click', hide);
  overlay.addEventListener('click', (event) => {
    if (event.target === overlay || event.target.closest('.viewer-overlay')) hide();
  });
  document.addEventListener('keydown', onKey);

  show(start);
  document.body.append(overlay);
  // The page under the dimmer stays where it is.
  document.body.classList.add('viewer-open');
}
