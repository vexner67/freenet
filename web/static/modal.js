// openvk's message box: a title bar, a body and buttons at the bottom,
// over a darkened page. Esc or a click beside the box closes it.

import { el, onScreen } from './layout.js';
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
// openMenu is the list of actions of a computer: under anchor, aligned
// to its right edge; it closes on a choice, a click aside, Esc and a
// second click on anchor.
let openedMenu = null;

export function openMenu(anchor, items) {
  const again = openedMenu?.anchor === anchor;
  openedMenu?.close();
  if (again) return;

  const list = el('div', { className: 'drop-menu', role: 'menu' }, ...items.map(({ text, run, danger = false }) => el('button', {
    type: 'button',
    className: danger ? 'drop-item danger' : 'drop-item',
    role: 'menuitem',
    textContent: text,
    onclick: () => {
      close();
      run();
    },
  })));

  function onKey(event) {
    if (event.key === 'Escape') close();
  }

  function onClick(event) {
    if (!list.contains(event.target) && !anchor.contains(event.target)) close();
  }

  function close() {
    list.remove();
    anchor.classList.remove('drop-anchor');
    document.removeEventListener('keydown', onKey);
    document.removeEventListener('click', onClick, true);
    if (openedMenu?.anchor === anchor) openedMenu = null;
  }

  document.body.append(list);
  anchor.classList.add('drop-anchor');
  const r = anchor.getBoundingClientRect();
  const left = Math.max(8, Math.min(r.right - list.offsetWidth, document.documentElement.clientWidth - list.offsetWidth - 8));
  list.style.left = `${left + window.scrollX}px`;
  list.style.top = `${r.bottom + window.scrollY + 4}px`;
  document.addEventListener('keydown', onKey);
  document.addEventListener('click', onClick, true);
  openedMenu = { anchor, close };
}

export function toast(text) {
  document.querySelector('.toast')?.remove();

  const note = el('div', { className: 'toast', role: 'status', textContent: text });
  document.body.append(note);
  setTimeout(() => note.remove(), 2200);
}

// notify leaves the page where it is after an action: what was done is
// said in node over the page and, when that is out of sight, at the
// bottom of the window too, instead of throwing the reader up to it.
export function notify(node, text = node.textContent) {
  if (text && !onScreen(node)) toast(text);
}
