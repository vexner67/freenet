// The texts the site translates (i18n.js): the keys of tr() in the
// scripts, the forms of plural() and the texts of the pages. A small
// tokenizer of JavaScript, enough for the site's scripts: strings,
// templates, comments and regular expressions are told apart.
import { readFileSync, readdirSync } from 'node:fs';

export const STATIC = new URL('../static/', import.meta.url);
export const WEB = new URL('../', import.meta.url);

const CYRILLIC = /[А-Яа-яЁё]/;
const REGEX_BEFORE = new Set('(,=:[!&|?{};+-*%<>~^'.split(''));
const REGEX_KEYWORDS = new Set(['return', 'typeof', 'case', 'in', 'of', 'new', 'delete', 'void', 'throw', 'else', 'do', 'yield', 'await']);

const ESCAPES = { n: '\n', t: '\t', r: '\r', b: '\b', f: '\f', v: '\v', 0: '\0' };

function cook(raw) {
  return raw.replace(/\\(u\{[0-9a-fA-F]+\}|u[0-9a-fA-F]{4}|x[0-9a-fA-F]{2}|\n|.)/g, (_, e) => {
    if (e[0] === 'u') return String.fromCodePoint(parseInt(e.replace(/[u{}]/g, ''), 16));
    if (e[0] === 'x') return String.fromCharCode(parseInt(e.slice(1), 16));
    if (e === '\n') return '';
    return ESCAPES[e] ?? e;
  });
}

// tokens are the tokens of a script: {type, value, cyrillic}; a template
// is one token, its expressions — tokens of their own after it.
export function tokens(src) {
  const out = [];
  let i = 0;
  let off = false;

  const last = () => out.findLast((t) => t.type !== 'template-part');
  const regexAllowed = () => {
    const t = last();
    if (!t) return true;
    if (t.type === 'ident') return REGEX_KEYWORDS.has(t.value);
    return t.type === 'punct' && REGEX_BEFORE.has(t.value);
  };

  const scan = (closer) => {
    let depth = 0;
    while (i < src.length) {
      const c = src[i];
      if (/\s/.test(c)) { i++; continue; }
      if (src.startsWith('//', i)) {
        const end = src.indexOf('\n', i);
        const comment = src.slice(i, end < 0 ? src.length : end);
        if (/i18n: off/.test(comment)) off = true;
        if (/i18n: on/.test(comment)) off = false;
        i = end < 0 ? src.length : end;
        continue;
      }
      if (src.startsWith('/*', i)) { i = src.indexOf('*/', i) + 2; continue; }
      if (c === "'" || c === '"') {
        let j = i + 1;
        while (src[j] !== c) j += src[j] === '\\' ? 2 : 1;
        const value = cook(src.slice(i + 1, j));
        out.push({ type: 'string', value, off, cyrillic: CYRILLIC.test(value) });
        i = j + 1;
        continue;
      }
      if (c === '`') {
        const token = { type: 'template', statics: [], off };
        out.push(token);
        i++;
        let start = i;
        for (;;) {
          if (src[i] === '\\') { i += 2; continue; }
          if (src[i] === '`') { token.statics.push(src.slice(start, i)); i++; break; }
          if (src[i] === '$' && src[i + 1] === '{') {
            token.statics.push(src.slice(start, i));
            i += 2;
            out.push({ type: 'template-part' });
            scan('}');
            i++;
            start = i;
            continue;
          }
          i++;
        }
        token.cyrillic = token.statics.some((s) => CYRILLIC.test(s));
        continue;
      }
      if (c === '/' && regexAllowed()) {
        let j = i + 1;
        let inClass = false;
        for (;;) {
          if (src[j] === '\\') { j += 2; continue; }
          if (src[j] === '[') inClass = true;
          else if (src[j] === ']') inClass = false;
          else if (src[j] === '/' && !inClass) break;
          j++;
        }
        j++;
        while (/[a-z]/.test(src[j] ?? '')) j++;
        out.push({ type: 'regex', value: src.slice(i, j) });
        i = j;
        continue;
      }
      if (/[A-Za-z_$0-9]/.test(c)) {
        let j = i;
        while (j < src.length && /[A-Za-z_$0-9]/.test(src[j])) j++;
        out.push({ type: 'ident', value: src.slice(i, j) });
        i = j;
        continue;
      }
      if ('([{'.includes(c)) depth++;
      if (')]}'.includes(c)) {
        if (depth === 0 && c === closer) return;
        depth--;
      }
      out.push({ type: 'punct', value: c });
      i++;
    }
  };

  scan(null);
  return out;
}

// scriptTexts are the texts of a script: the keys of tr(), the forms of
// plural() and the Russian left outside them.
export function scriptTexts(src) {
  const list = tokens(src);
  const keys = [];
  const plurals = [];
  const loose = [];
  const used = new Set();

  list.forEach((t, k) => {
    if (t.type === 'ident' && t.value === 'tr' && list[k + 1]?.value === '(' && list[k + 2]?.type === 'string') {
      keys.push(list[k + 2].value);
      used.add(k + 2);
    }
    if (t.type === 'ident' && t.value === 'plural' && list[k + 1]?.value === '(') {
      let j = k + 2;
      while (j < list.length && !(list[j].value === '[' && list[j - 1].value === ',')) j++;
      const forms = [];
      for (j++; list[j]?.type === 'string'; j += 2) {
        forms.push(list[j].value);
        used.add(j);
        if (list[j + 1]?.value !== ',') break;
      }
      if (forms.length) plurals.push(forms.join('|'));
    }
  });

  list.forEach((t, k) => {
    if (t.cyrillic && !t.off && !used.has(k)) loose.push(t.value ?? t.statics.join('${…}'));
  });

  return { keys, plurals, loose };
}

const ENTITIES = { raquo: '»', laquo: '«', nbsp: ' ', mdash: '—', ndash: '–', amp: '&', lt: '<', gt: '>', quot: '"', hellip: '…', copy: '©', middot: '·', bull: '•', apos: "'" };

function decode(s) {
  return s.replace(/&(#x?[0-9a-fA-F]+|\w+);/g, (whole, e) => {
    if (e[0] === '#') return String.fromCodePoint(e[1] === 'x' ? parseInt(e.slice(2), 16) : Number(e.slice(1)));
    return ENTITIES[e] ?? whole;
  });
}

const pageText = (s) => s.replace(/\s+/g, ' ').trim();

// pageTexts are the Russian texts of a page as i18n.js translates them:
// its title before " | ", the text between its tags and its attributes.
export function pageTexts(html) {
  const texts = [];
  const title = html.match(/<title>([^<]*)<\/title>/)?.[1];
  if (title && CYRILLIC.test(title)) texts.push(pageText(decode(title.split(' | ')[0])));

  const body = html.slice(html.indexOf('<body'));
  for (const [, text] of body.matchAll(/>([^<]+)</g)) {
    if (CYRILLIC.test(text)) texts.push(pageText(decode(text)));
  }
  for (const [, , value] of body.matchAll(/\s(placeholder|title|aria-label|alt)="([^"]*)"/g)) {
    if (CYRILLIC.test(value)) texts.push(pageText(decode(value)));
  }
  return texts;
}

// SCRIPTS are the modules of the site that translate with tr().
export const SCRIPTS = readdirSync(STATIC).filter((f) => f.endsWith('.js') && !['en.js', 'theme.js', 'sw.js'].includes(f));
// PAGES are the pages of a computer and those of a phone (m/).
export const PAGES = [
  ...readdirSync(WEB).filter((f) => f.endsWith('.html')),
  ...readdirSync(new URL('m/', WEB)).filter((f) => f.endsWith('.html')).map((f) => `m/${f}`),
];

// allTexts are every text of the site that en.js must have.
export function allTexts() {
  const texts = new Map();
  const add = (text, where) => { if (!texts.has(text)) texts.set(text, where); };

  for (const file of SCRIPTS) {
    const { keys, plurals } = scriptTexts(readFileSync(new URL(file, STATIC), 'utf8'));
    for (const key of [...keys, ...plurals]) add(key, file);
  }
  for (const file of PAGES) {
    for (const text of pageTexts(readFileSync(new URL(file, WEB), 'utf8'))) add(text, file);
  }
  return texts;
}
