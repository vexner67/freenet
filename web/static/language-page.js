// openvk's "Выбрать язык" (/language): every language with its flag.
// /language?lg=<code>, the address of a language here and in the footer,
// switches to it and comes back.

import { currentUser } from './api.js';
import { LANGUAGES, setLanguage } from './i18n.js';
import { el, renderChrome } from './layout.js';

const chosen = new URLSearchParams(location.search).get('lg');
if (chosen) {
  setLanguage(chosen);
  location.replace('/language');
} else {
  renderChrome(await currentUser().catch(() => null));

  document.getElementById('languages').replaceChildren(...LANGUAGES.map((language) => el('a', {
    className: 'language',
    href: `/language?lg=${language.code}`,
    rel: 'nofollow',
  }, el('img', { src: language.flag, alt: language.name }), el('br'), language.name)));
}
