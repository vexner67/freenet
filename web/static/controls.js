// The controls of the games of money — "Бизнес" and "Акции" — as the
// browser games of the 2000s had them: a mini window that opens under what
// was clicked (popover), a number with "−" and "+" (stepper), a row of
// quick amounts (chips), a slider and a switch of a few choices
// (segmented). They hold numbers; the pages say what the numbers mean.

import { el } from './layout.js';

let open = null;

function closeOpen() {
  open?.close();
}

document.addEventListener('mousedown', (event) => {
  if (open && !open.box.contains(event.target) && !open.anchor.contains(event.target)) closeOpen();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape') closeOpen();
});
window.addEventListener('resize', closeOpen);

// popover opens a mini window titled heading under anchor, closing the one
// open before; a second click on the anchor closes it. body is filled by
// the caller and may be refilled; error is its place for errors.
export function popover(anchor, heading) {
  if (open?.anchor === anchor) {
    closeOpen();
    return null;
  }
  closeOpen();

  const error = el('div', { className: 'error', hidden: true });
  const body = el('div', { className: 'pop-body' });
  const box = el('div', { className: 'pop', role: 'dialog' },
    el('div', { className: 'pop-head' },
      el('span', { textContent: heading }),
      el('button', { type: 'button', className: 'pop-close', textContent: '×', onclick: () => closeOpen() })),
    error, body);
  document.body.append(box);
  anchor.classList.add('pop-anchor');

  const place = () => {
    const r = anchor.getBoundingClientRect();
    const width = Math.min(box.offsetWidth, document.documentElement.clientWidth - 16);
    const left = Math.min(Math.max(8, r.left), document.documentElement.clientWidth - width - 8);
    box.style.left = `${left + window.scrollX}px`;
    box.style.top = `${r.bottom + window.scrollY + 4}px`;
  };

  const self = {
    anchor, box, body, error, place,
    close() {
      box.remove();
      anchor.classList.remove('pop-anchor');
      if (open === self) open = null;
    },
  };
  open = self;
  requestAnimationFrame(place);
  return self;
}

// isOpen is whether a mini window is open: the pages do not redraw under
// it.
export function isOpen() {
  return open !== null;
}

// stepper is a whole number from min to max with "−" and "+" a step at a
// time; onchange hears every change. Its value is read and set with
// get and set.
export function stepper({ value = 0, min = 0, max = Infinity, step = 1, onchange = () => {}, wide = false }) {
  const input = el('input', { type: 'text', inputMode: 'numeric', className: wide ? 'step-value wide' : 'step-value', value: String(value) });
  const clamp = (n) => Math.min(Math.max(Number.isFinite(n) ? Math.round(n) : min, min), max);
  const set = (n, quiet = false) => {
    input.value = String(clamp(n));
    if (!quiet) onchange(clamp(n));
  };
  const get = () => clamp(Number(input.value.replace(/\s/g, '')));
  const press = (delta) => el('button', {
    type: 'button', className: 'step-button', textContent: delta < 0 ? '−' : '+', onclick: () => set(get() + delta * self.step),
  });

  input.addEventListener('input', () => onchange(get()));
  input.addEventListener('change', () => set(get()));
  const self = {
    node: el('span', { className: 'stepper' }, press(-1), input, press(1)),
    get, set, step,
    limit(lo, hi) {
      min = lo;
      max = hi;
      set(get(), true);
    },
  };
  return self;
}

// chips are quick choices: [label, what it does] in a row.
export function chips(list) {
  return el('span', { className: 'chips' }, ...list.filter(Boolean).map(([label, onclick, className = '']) => el('button', {
    type: 'button', className: `chip ${className}`.trim(), textContent: label, onclick,
  })));
}

// slider is a range from min to max with what it means shown beside it.
export function slider({ value, min, max, step = 1, onchange = () => {} }) {
  const input = el('input', { type: 'range', className: 'slider', min: String(min), max: String(max), step: String(step), value: String(value) });
  input.addEventListener('input', () => onchange(Number(input.value)));
  return { node: input, get: () => Number(input.value), set: (n) => { input.value = String(n); onchange(Number(input.value)); } };
}

// segmented is a switch of choices [value, label, class]; onchange hears
// the value chosen.
export function segmented(choices, value, onchange = () => {}) {
  const node = el('span', { className: 'segmented' });
  let current = value;
  const draw = () => node.replaceChildren(...choices.map(([v, label, className = '']) => el('button', {
    type: 'button', className: `segment ${className} ${v === current ? 'active' : ''}`.trim(), textContent: label,
    onclick: () => {
      current = v;
      draw();
      onchange(v);
    },
  })));
  draw();
  return { node, get: () => current };
}

// meter is a strip filled to part of whole, with its text over it; tone is
// a class of its fill.
export function meter(part, whole, text = '', tone = '') {
  const fill = el('span', { className: `meter-fill ${tone}`.trim() });
  fill.style.width = `${whole > 0 ? Math.min(100, Math.max(0, (part / whole) * 100)) : 0}%`;
  return el('span', { className: 'meter' }, fill, el('span', { className: 'meter-text', textContent: text }));
}
