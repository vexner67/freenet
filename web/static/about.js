// "О сайте", openvk's "Об этой инстанции": what the site is for, how many
// people, groups and posts it has, and where its rules are.

import { api, currentUser } from './api.js';
import { aboutStatistics } from './labels.js';
import { $, el, renderChrome, showError } from './layout.js';

renderChrome(await currentUser().catch(() => null));

try {
  const counts = await api.about();

  // openvk writes the number in bold before the words.
  $('statistics').replaceChildren(...aboutStatistics(counts).map(([n, words]) =>
    el('li', {}, el('b', { textContent: String(n) }), ' ' + words)));
} catch (e) {
  showError($('error'), e);
}
