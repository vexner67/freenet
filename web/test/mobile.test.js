import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { activeSection } from '../static/labels.js';

const web = new URL('../', import.meta.url);
const pages = readdirSync(web).filter((name) => name.endsWith('.html'));

test('every page is laid out for the width of a phone and names the app manifest', () => {
  for (const page of pages) {
    const html = readFileSync(new URL(page, web), 'utf8');

    assert.match(html, /<meta name="viewport" content="width=device-width, initial-scale=1">/, page);
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

test('every page of the site loads the interface of a phone, for a narrow screen only', () => {
  for (const page of pages.filter((name) => name !== 'admin.html')) {
    const html = readFileSync(new URL(page, web), 'utf8');

    assert.match(html, /<link rel="stylesheet" href="\/static\/style\.css">\n\s*<link rel="stylesheet" href="\/static\/mobile\.css" media="\(max-width: 800px\)">/, page);
  }
});

test('the interface of a phone keeps to its stylesheet', () => {
  const style = readFileSync(new URL('static/style.css', web), 'utf8');

  assert.doesNotMatch(style, /@media \(max-width: 800px\)/, 'a rule of the phone belongs in mobile.css');
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
