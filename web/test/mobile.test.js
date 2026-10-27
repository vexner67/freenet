import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { activeSection } from '../static/labels.js';

const web = new URL('../', import.meta.url);
const pages = readdirSync(web).filter((name) => name.endsWith('.html'));

test('every page names the app manifest and lets a tablet see it whole', () => {
  for (const page of pages.filter((name) => name !== 'admin.html')) {
    const html = readFileSync(new URL(page, web), 'utf8');

    // A page of a computer is 791px wide: a tablet shows it all, scaled.
    assert.match(html, /<meta name="viewport" content="width=820">/, page);
    assert.match(html, /<link rel="manifest" href="\/static\/manifest\.json">/, page);
  }
});

test('the icons of the app manifest are files of the site', () => {
  const manifest = JSON.parse(readFileSync(new URL('static/manifest.json', web), 'utf8'));

  assert.equal(manifest.start_url, '/');
  assert.ok(manifest.icons.some((icon) => icon.sizes === '512x512' && icon.purpose === 'maskable'));
  for (const icon of manifest.icons) {
    assert.ok(existsSync(new URL('.' + icon.src, web)), icon.src);
  }
});

test('every page but the admin panel has its page of a phone', () => {
  for (const page of pages.filter((name) => name !== 'admin.html')) {
    assert.ok(existsSync(new URL(`m/${page}`, web)), page);
  }
});

test('the interface of a phone keeps to its stylesheet', () => {
  const style = readFileSync(new URL('static/style.css', web), 'utf8');

  assert.doesNotMatch(style, /@media \(max-width: 800px\)/, 'a rule of the phone belongs in m.css');
  assert.match(style, /\.tabbar, \.header-search \{ display: none; \}/, 'the bar of sections is the phone\'s only');
});

test('the icons of the bar of sections are files of the site', () => {
  for (const name of ['tab-page', 'tab-feed', 'tab-messages', 'tab-notifications', 'tab-menu', 'search']) {
    assert.ok(existsSync(new URL(`static/icons/${name}.svg`, web)), name);
  }
});

test('the bar of sections lights the section the page is in', () => {
  assert.equal(activeSection('/feed', '/ann'), 'feed');
  assert.equal(activeSection('/feed/all', '/ann'), 'feed');
  assert.equal(activeSection('/im', '/ann'), 'messages');
  assert.equal(activeSection('/im/petr', '/ann'), 'messages');
  assert.equal(activeSection('/im/chat/1', '/ann'), 'messages');
  assert.equal(activeSection('/notifications', '/ann'), 'notifications');
  assert.equal(activeSection('/ann', '/ann'), 'page');
  assert.equal(activeSection('/petr', '/ann'), '');
  assert.equal(activeSection('/imogen', '/ann'), '');
  assert.equal(activeSection('/feedback', '/ann'), '');
  assert.equal(activeSection('/ann', ''), '');
});

// The pages of a phone (web/m/), which the gateway gives a phone instead
// of the page of the same name: their own markup over the same scripts.
const phone = new URL('m/', web);
const phonePages = readdirSync(phone).filter((name) => name.endsWith('.html'));
const ids = (html) => [...html.matchAll(/\sid="([^"]+)"/g)].map((m) => m[1]).sort();
const modules = (html) => [...html.matchAll(/<script type="module" src="([^"]+)"/g)].map((m) => m[1]);

test('every page of a phone has a page of a computer and wears m.css alone', () => {
  assert.ok(phonePages.length > 0);
  for (const page of phonePages) {
    const html = readFileSync(new URL(page, phone), 'utf8');

    assert.ok(pages.includes(page), page);
    assert.match(html, /<html lang="ru" class="m">/, page);
    assert.match(html, /<meta name="viewport" content="width=device-width, initial-scale=1">/, page);
    assert.match(html, /<link rel="manifest" href="\/static\/manifest\.json">/, page);
    assert.match(html, /<link rel="stylesheet" href="\/static\/palette\.css">\n\s*<link rel="stylesheet" href="\/static\/m\.css">\n\s*<script src="\/static\/theme\.js"><\/script>\n\s*<script type="module"/, page);
    assert.doesNotMatch(html, /style\.css|mobile\.css/, page);
  }
});

test('a page of a phone has the elements of its page of a computer, for the same scripts', () => {
  for (const page of phonePages) {
    const html = readFileSync(new URL(page, phone), 'utf8');
    const desktop = readFileSync(new URL(page, web), 'utf8');

    assert.deepEqual(ids(html), ids(desktop), page);
    assert.deepEqual(modules(html), modules(desktop), page);
  }
});
