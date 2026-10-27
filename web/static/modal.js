// openvk's message box: a title bar, a body and buttons at the bottom,
// over a darkened page. Esc or a click beside the box closes it.

import { el } from './layout.js';
import { tr } from './i18n.js';

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

// openSheet is a list of what may be done, over the bottom of the screen
// of a phone, as its apps have it: items are { text, run, danger }; a tap
// on one closes the list and does it, "Отмена", Esc or a tap beside the
// list only close it.
export function openSheet(items) {
  const overlay = el('div', { className: 'sheet' });

  function onKey(event) {
    if (event.key === 'Escape') close();
  }

  function close() {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  }

  overlay.append(el('div', { className: 'sheet-box', role: 'menu' },
    el('div', { className: 'sheet-list' }, ...items.map(({ text, run, danger = false }) => el('button', {
      type: 'button',
      className: danger ? 'sheet-item danger' : 'sheet-item',
      role: 'menuitem',
      textContent: text,
      onclick: () => {
        close();
        run();
      },
    }))),
    el('button', { type: 'button', className: 'sheet-item sheet-cancel', textContent: tr('Отмена'), onclick: close })));
  overlay.addEventListener('click', (event) => {
    if (event.target === overlay) close();
  });
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
}

// toast says in a word over the bottom of the screen what was done, and
// goes by itself.
export function toast(text) {
  document.querySelector('.toast')?.remove();

  const note = el('div', { className: 'toast', role: 'status', textContent: text });
  document.body.append(note);
  setTimeout(() => note.remove(), 2200);
}
