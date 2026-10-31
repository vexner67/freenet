// What makes the admin panel answer: the sections one press and a few letters
// away, numbers that count up, a saved form that flies in over the page with
// a sound of coins, the fields changed and not saved yet in yellow, and the
// deeds of the day in the header. The pages of admin.js need not know: it is
// enough that they show their messages. Everything that moves keeps still
// under reduced motion, and the numbers just change.

import { coins, dealTick, fanfare } from './sounds.js';
import { tr } from './i18n.js';

const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
const toastTime = 3_000;
const countTime = 600;
const deedsKey = 'freenet.admindeeds';
// The deeds of a day that a fanfare salutes.
const milestones = [5, 10, 25, 50, 100];

// countUp runs the number of node up or down to value; a change on an open
// page lights it.
const values = new WeakMap();

export function countUp(node, value, text) {
  const from = values.get(node);

  values.set(node, value);
  if (from === value) return;
  if (from !== undefined) bump(node.closest('.admin-card'));
  if (still) {
    node.textContent = text(value);
    return;
  }

  const start = performance.now();
  const step = (now) => {
    const part = Math.min(1, (now - start) / countTime);

    node.textContent = text(Math.round((from ?? 0) + (value - (from ?? 0)) * (1 - (1 - part) ** 3)));
    if (part < 1 && values.get(node) === value) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}

function bump(node) {
  if (!node) return;

  node.classList.remove('admin-bump');
  void node.offsetWidth; // the animation starts over
  node.classList.add('admin-bump');
}

function today() {
  return new Date().toISOString().slice(0, 10);
}

function savedDeeds() {
  try {
    const kept = JSON.parse(localStorage.getItem(deedsKey));

    return kept?.day === today() ? kept.count : 0;
  } catch {
    return 0;
  }
}

function showDeeds(count) {
  document.getElementById('deeds-count').textContent = String(count);
  document.getElementById('deeds').hidden = count === 0;
  document.getElementById('deeds').title = tr('Сделано сегодня: {0}', count);
}

function fly(from) {
  if (still || !from) return;

  const box = from.getBoundingClientRect();
  const mark = document.createElement('div');

  mark.className = 'admin-float';
  mark.textContent = '+1';
  mark.style.left = `${box.left + box.width / 2}px`;
  mark.style.top = `${box.top}px`;
  mark.addEventListener('animationend', () => mark.remove());
  document.body.append(mark);
}

// deed counts an action done; from is what was pressed.
export function deed(from = document.activeElement) {
  const count = savedDeeds() + 1;

  try {
    localStorage.setItem(deedsKey, JSON.stringify({ day: today(), count }));
  } catch {
    // The count just stays where it was.
  }
  showDeeds(count);
  bump(document.getElementById('deeds'));
  fly(from);
  if (milestones.includes(count)) fanfare();
  else coins();
}

// A message of success comes as a toast and goes by itself; the voucher just
// created stays, as its code is to be copied.
const hideTimers = new WeakMap();

function heard(message) {
  if (message.hidden) return;

  clearTimeout(hideTimers.get(message));
  deed(lastPress);
  for (const field of document.querySelectorAll('.admin-dirty')) field.classList.remove('admin-dirty');
  if (message.id !== 'created') hideTimers.set(message, setTimeout(() => { message.hidden = true; }, toastTime));
}

let lastPress = null;

function watchMessages() {
  const watcher = new MutationObserver((changes) => changes.forEach((change) => heard(change.target)));

  for (const message of document.querySelectorAll('.aui-message-success')) watcher.observe(message, { attributes: true, attributeFilter: ['hidden'] });
}

function showNavFilter() {
  const filter = document.getElementById('nav-filter');
  const items = [...document.querySelectorAll('.aui-nav-item')];

  filter.addEventListener('input', () => {
    const words = filter.value.trim().toLowerCase();

    for (const item of items) item.hidden = !item.textContent.toLowerCase().includes(words);
    for (const group of document.querySelectorAll('.aui-sidebar-group')) group.hidden = !group.querySelector('.aui-nav-item:not([hidden])');
  });
  filter.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') items.find((item) => !item.hidden)?.click();
  });
}

function launcher(href, iconClass, text) {
  const link = document.createElement('a');
  const icon = document.createElement('span');

  link.className = 'admin-launch';
  link.href = href;
  icon.className = iconClass;
  link.append(icon, text);

  return link;
}

// The tiles of "Сводка": every section and the two things made most often.
function showLaunchers() {
  const tiles = [...document.querySelectorAll('.aui-nav-item')]
    .filter((item) => item.getAttribute('href').startsWith('/admin/'))
    .map((item) => launcher(item.getAttribute('href'), item.querySelector('.aui-icon').className, item.textContent));

  document.getElementById('launchers').append(
    ...tiles,
    launcher('/admin/vouchers/new', 'aui-icon aui-icon-card', tr('Новый ваучер')),
    launcher('/admin/ads/new', 'aui-icon aui-icon-ad', tr('Новая реклама')),
  );
}

// Each heading wears the icon of its section.
function showHeadingIcons() {
  const icon = document.querySelector('.aui-nav-item.active .aui-icon');
  if (!icon) return;

  for (const heading of document.querySelectorAll('h1.aui-page-heading')) heading.prepend(icon.cloneNode());
}

function listen() {
  document.addEventListener('click', (event) => {
    const pressed = event.target.closest?.('button, .aui-button');

    if (pressed) lastPress = pressed;
    if (event.target.closest?.('#switch-theme')) dealTick();
  }, true);
  document.addEventListener('submit', (event) => {
    lastPress = event.submitter ?? lastPress;
    dealTick();
  }, true);
  document.addEventListener('input', (event) => {
    if (event.target.closest('form.aui:not(.admin-search)')) event.target.closest('.field-group')?.classList.add('admin-dirty');
  });
  document.addEventListener('keydown', (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
      const form = document.activeElement?.closest?.('form.aui') ?? document.querySelector('.admin-dirty')?.closest('form.aui');

      event.preventDefault();
      form?.requestSubmit();
    }
    if (event.key === '/' && !event.target.closest?.('input, textarea, select')) {
      event.preventDefault();
      document.getElementById('nav-filter').focus();
    }
  });
}

// lookReady is called when the links of the sidebar are lit.
export function lookReady() {
  showDeeds(savedDeeds());
  showNavFilter();
  showLaunchers();
  showHeadingIcons();
  watchMessages();
  listen();
}
