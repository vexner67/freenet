import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';

import { safeURL } from '../static/labels.js';

const web = new URL('../', import.meta.url);
const statics = new URL('../static/', import.meta.url);

const scripts = readdirSync(statics).filter((name) => name.endsWith('.js'));
const pages = [
  ...readdirSync(web).filter((name) => name.endsWith('.html')),
  ...readdirSync(new URL('m/', web)).filter((name) => name.endsWith('.html')).map((name) => `m/${name}`),
];

// What turns a string into markup or code. Everything a person wrote is
// put on a page with textContent and el(); theme.js alone writes markup,
// the link of a theme, through its Trusted Types policy.
const sinks = [
  /\.innerHTML\b/, /\.outerHTML\b/, /\binnerHTML\s*:/, /insertAdjacentHTML/, /setHTMLUnsafe/, /\bsrcdoc\b/,
  /createContextualFragment/, /parseFromString/, /\beval\s*\(/, /new Function\s*\(/,
  /set(Timeout|Interval)\(\s*['"`]/, /document\.write/,
];

test('scripts make no markup of strings', () => {
  for (const name of scripts) {
    const code = readFileSync(new URL(name, statics), 'utf8');
    for (const sink of sinks) {
      if (name === 'theme.js' && sink.source === 'document\\.write') continue;
      assert.doesNotMatch(code, sink, `${name} uses ${sink}`);
    }
  }

  assert.match(readFileSync(new URL('theme.js', statics), 'utf8'), /document\.write\(policy \? policy\.createHTML\(markup\) : markup\)/);
});

test('pages carry no inline code or styles', () => {
  for (const name of pages) {
    const html = readFileSync(new URL(name, web), 'utf8');
    assert.doesNotMatch(html, /<script(?![^>]*\bsrc=)[^>]*>/i, `${name} has an inline script`);
    assert.doesNotMatch(html, /\son[a-z]+\s*=/i, `${name} has a handler in an attribute`);
    assert.doesNotMatch(html, /\sstyle\s*=/i, `${name} has a style attribute`);
    assert.doesNotMatch(html, /javascript:/i, `${name} links to a script`);
  }
});

test('safeURL lets no script through a link', () => {
  for (const url of ['/id1', 'post/1', '#top', '?q=1', 'https://example.com/a?b=c', 'http://example.com',
    'mailto:ann@example.com', 'tel:+79990000000', 'blob:https://freenet19.fun/1', 'data:image/png;base64,AAAA', '']) {
    assert.equal(safeURL(url), url, url);
  }

  for (const url of ['javascript:alert(1)', 'JavaScript:alert(1)', ' javascript:alert(1)', '\u0001javascript:alert(1)',
    'java\tscript:alert(1)', 'java\nscript:alert(1)', 'vbscript:msgbox(1)', 'data:text/html,<script>alert(1)</script>',
    'data:image/svg+xml;base64,PHN2Zz4=', 'file:///etc/passwd']) {
    assert.equal(safeURL(url), 'about:invalid', url);
  }
});
