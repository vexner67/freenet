// openvk's message box: a title bar, a body and buttons at the bottom,
// over a darkened page. Esc or a click beside the box closes it.

import { el } from './layout.js';

// openModal shows a box and returns it: body and actions are its parts to
// fill (and refill), close removes it.
export function openModal(title, { tag = 'div' } = {}) {
  const body = el('div', { className: 'modal-body' });
  const actions = el('div', { className: 'modal-actions' });
  const box = el(tag, { className: 'modal-box' }, el('div', { className: 'modal-header', textContent: title }), body, actions);
  const overlay = el('div', { className: 'modal' }, box);

  function onKey(event) {
    if (event.key === 'Escape') close();
  }

  function close() {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  }

  overlay.addEventListener('click', (event) => {
    if (event.target === overlay) close();
  });
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);

  return { box, body, actions, close };
}

// button is a box button; secondary ones are grey, as "Отмена".
export function button(text, onclick, { secondary = false, type = 'button' } = {}) {
  return el('button', { className: secondary ? 'button secondary' : 'button', type, textContent: text, onclick });
}
