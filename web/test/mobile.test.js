import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';

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
