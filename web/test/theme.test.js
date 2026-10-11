import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('../static/theme.js', import.meta.url), 'utf8');
const script = 'https://freenet.test/static/v/abc123/theme.js';

// written is what theme.js writes into a document whose storage holds items.
function written(items, broken = false) {
  const out = [];

  vm.runInNewContext(source, {
    URL,
    localStorage: {
      getItem(key) {
        if (broken) throw new Error('storage is blocked');

        return items[key] ?? null;
      },
    },
    document: {
      currentScript: { src: script },
      write: (html) => out.push(html),
    },
  });

  return out.join('');
}

test('the theme of the settings is put on beside the script', () => {
  const html = written({ 'freenet.interface': JSON.stringify({ theme: 'dark' }) });

  assert.equal(html, '<link id="theme" rel="stylesheet" href="https://freenet.test/static/v/abc123/themes/dark.css">');
});

test('the theme of the device wins over the one of the settings', () => {
  const html = written({ 'freenet.theme': 'blue', 'freenet.interface': JSON.stringify({ theme: 'dark' }) });

  assert.match(html, /themes\/blue\.css/);
});

test('nothing is written for the default theme, an unknown one or broken storage', () => {
  assert.equal(written({}), '');
  assert.equal(written({ 'freenet.interface': JSON.stringify({ theme: 'default' }) }), '');
  assert.equal(written({ 'freenet.theme': '"><script src=//evil>' }), '');
  assert.equal(written({ 'freenet.interface': '{not json' }), '');
  assert.equal(written({ 'freenet.theme': 'dark' }, true), '');
});

test('every page runs the script right after style.css, before its modules', () => {
  const dir = new URL('../', import.meta.url);
  const pages = readdirSync(dir).filter((name) => name.endsWith('.html'));

  assert.ok(pages.length > 30);

  for (const name of pages) {
    const html = readFileSync(new URL(name, dir), 'utf8');

    assert.match(
      html,
      /<link rel="stylesheet" href="\/static\/style\.css">\n\s*<script src="\/static\/theme\.js"><\/script>\n\s*<script type="module"/,
      name,
    );
  }
});
