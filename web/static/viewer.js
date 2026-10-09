// The photo viewer, as openvk's: the large photo over a dark page with
// "Фотография N из M", arrows between the photos of a post and "Закрыть".
// Esc closes, ← and → page. details(photo), if given, is shown under the
// photo: for a photo of an album its description, heart and comments.

import { el } from './layout.js';

export function openViewer(photos, start, { details } = {}) {
  let index = start;

  const counter = el('span');
  const img = el('img', { className: 'viewer-photo', alt: '' });
  const prev = el('a', { className: 'viewer-nav viewer-prev', title: 'Предыдущая', textContent: '‹' });
  const next = el('a', { className: 'viewer-nav viewer-next', title: 'Следующая', textContent: '›' });
  const close = el('a', { className: 'viewer-close', textContent: 'Закрыть' });

  const footer = el('div', { className: 'viewer-details', hidden: !details });
  const box = el('div', { className: 'viewer-box' },
    el('div', { className: 'viewer-header' }, counter, close),
    el('div', { className: 'viewer-body' }, prev, img, next),
    footer,
  );
  const overlay = el('div', { className: 'viewer' }, box);

  function show(i) {
    index = (i + photos.length) % photos.length;
    img.src = photos[index].large;
    counter.textContent = `Фотография ${index + 1} из ${photos.length}`;
    if (details) footer.replaceChildren(details(photos[index]));
  }

  function onKey(event) {
    if (event.key === 'Escape') hide();
    if (event.key === 'ArrowLeft' && photos.length > 1) show(index - 1);
    if (event.key === 'ArrowRight' && photos.length > 1) show(index + 1);
  }

  function hide() {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  }

  prev.hidden = next.hidden = photos.length < 2;
  prev.addEventListener('click', () => show(index - 1));
  next.addEventListener('click', () => show(index + 1));
  // A click on the photo shows the next one, as on the old sites.
  img.addEventListener('click', () => photos.length > 1 ? show(index + 1) : hide());
  close.addEventListener('click', hide);
  overlay.addEventListener('click', (event) => {
    if (event.target === overlay) hide();
  });
  document.addEventListener('keydown', onKey);

  show(start);
  document.body.append(overlay);
}
