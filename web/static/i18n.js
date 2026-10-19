// The language of the site, as openvk's "Выбрать язык": Russian, as the
// site is written, or English. The language is the browser's choice
// (localStorage), as openvk keeps it in the session, and changing it
// reloads the page. Texts are translated as gettext does: the Russian
// text is the key, en.js has its English, and a text it lacks stays
// Russian. tr() translates a text of a script; the texts of the page
// itself are translated as the page loads.

// LANGUAGES are openvk's locales/list.yml as far as the site goes: the
// code, the flag (openvk's picture, a whole path so that it is versioned)
// and the name in the language.
export const LANGUAGES = [
  { code: 'ru', flag: '/static/flags/ru.gif', name: 'Русский' },
  { code: 'en', flag: '/static/flags/gb.gif', name: 'English' },
];

const KEY = 'freenet.lang';

function stored() {
  try {
    const code = globalThis.localStorage?.getItem(KEY);
    return LANGUAGES.some((l) => l.code === code) ? code : 'ru';
  } catch {
    return 'ru';
  }
}

// lang is the language of this page: 'ru' or 'en'.
export const lang = stored();

const dictionary = lang === 'en' ? (await import('/static/en.js')).default : null;

// tr is text in the language of the page; {0}, {1}… in it are args.
export function tr(text, ...args) {
  const shown = dictionary?.[text] ?? text;

  return args.length ? shown.replace(/\{(\d+)\}/g, (_, i) => String(args[i])) : shown;
}

// setLanguage keeps the language for the next pages of this browser.
export function setLanguage(code) {
  try {
    localStorage.setItem(KEY, code);
  } catch {
    // Private mode: the language lasts as long as the page.
  }
}

// pageText is a text of a page as a key: its spaces and line breaks
// collapsed, the ends trimmed.
export function pageText(s) {
  return s.replace(/\s+/g, ' ').trim();
}

const CYRILLIC = /[А-Яа-яЁё]/;
const ATTRIBUTES = ['placeholder', 'title', 'aria-label', 'alt'];

function translateText(node) {
  const key = pageText(node.nodeValue);
  const shown = dictionary[key];
  if (!shown) return;

  const [, before, , after] = node.nodeValue.match(/^(\s*)([\s\S]*?)(\s*)$/);
  node.nodeValue = before + shown + after;
}

// translatePage translates the texts and attributes the page came with,
// before its script draws anything.
function translatePage(doc) {
  doc.documentElement.lang = lang;
  if (!dictionary) return;

  const title = doc.title.split(' | ');
  title[0] = tr(title[0]);
  doc.title = title.join(' | ');

  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) => (CYRILLIC.test(node.nodeValue) ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT),
  });
  for (let node = walker.nextNode(); node; node = walker.nextNode()) translateText(node);

  for (const name of ATTRIBUTES) {
    for (const element of doc.body.querySelectorAll(`[${name}]`)) {
      const value = element.getAttribute(name);
      if (CYRILLIC.test(value)) element.setAttribute(name, tr(pageText(value)));
    }
  }
}

if (globalThis.document?.body) translatePage(document);
