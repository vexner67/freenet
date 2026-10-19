// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import en from '../static/en.js';
import { allTexts, scriptTexts, SCRIPTS, STATIC } from './i18n-keys.js';

const texts = allTexts();
const placeholders = (s) => new Set(s.match(/\{\d+\}/g) ?? []);

test('every text of the site has its English', () => {
  const missing = [...texts].filter(([text]) => !(text in en)).map(([text, file]) => `${file}: ${text}`);

  assert.deepEqual(missing, [], 'add these to static/en.js');
});

test('en.js has no texts the site no longer says', () => {
  const unused = Object.keys(en).filter((text) => !texts.has(text));

  assert.deepEqual(unused, [], 'remove these from static/en.js');
});

test('Russian in scripts goes through tr()', () => {
  // The names of languages are said in their own languages.
  const allowed = new Set(['Русский']);
  const loose = SCRIPTS.flatMap((file) => scriptTexts(readFileSync(new URL(file, STATIC), 'utf8')).loose
    .filter((text) => !allowed.has(text))
    .map((text) => `${file}: ${text}`));

  assert.deepEqual(loose, [], 'wrap these in tr() or mark the grammar with "// i18n: off" and "// i18n: on"');
});

test('an English text says only what its Russian gives it', () => {
  for (const [text, english] of Object.entries(en)) {
    const given = placeholders(text);
    for (const p of placeholders(english)) assert.ok(given.has(p), `${text} → ${english}: ${p}`);
  }
});

test('an English plural is the singular and the plural', () => {
  for (const [text, english] of Object.entries(en)) {
    if (!text.includes('|') || text.includes(' | ')) continue;
    assert.equal(english.split('|').length, 2, `${text} → ${english}`);
  }
});
