// What makes the admin panel answer: the sections a few letters away, the
// fields changed and not saved yet in yellow, and a message of success that
// goes by itself. The pages of admin.js need not know: it is enough that
// they show their messages.

import { tr } from './i18n.js';

const toastTime = 3_000;

// A message of success comes as a toast and goes by itself; the voucher just
// created stays, as its code is to be copied.
const hideTimers = new WeakMap();

function heard(message) {
  if (message.hidden) return;

  clearTimeout(hideTimers.get(message));
  for (const field of document.querySelectorAll('.admin-dirty')) field.classList.remove('admin-dirty');
  if (message.id !== 'created') hideTimers.set(message, setTimeout(() => { message.hidden = true; }, toastTime));
}

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
  showNavFilter();
  showLaunchers();
  showHeadingIcons();
  watchMessages();
  listen();
}
