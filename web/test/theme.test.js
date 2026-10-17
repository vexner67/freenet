import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('../static/theme.js', import.meta.url), 'utf8');

// run runs theme.js in a document whose storage holds items: what it
// writes, the classes of the page and what it leaves for later.
function run(items, broken = false) {
  const out = [];
  const classes = new Set();
  const later = [];

  vm.runInNewContext(source, {
    URL,
    localStorage: {
      getItem(key) {
        if (broken) throw new Error('storage is blocked');

        return items[key] ?? null;
      },
    },
    Object,
    setTimeout: (fn, ms) => later.push({ fn, ms }),
    document: {
      write: (html) => out.push(html),
      documentElement: { classList: { add: (c) => classes.add(c), remove: (c) => classes.delete(c) } },
    },
  });

  return { html: out.join(''), classes, later };
}

// written is what theme.js writes into a document whose storage holds items.
const written = (items, broken) => run(items, broken).html;

test('the page is hidden until it is drawn, and shown anyway after a while', () => {
  const { classes, later } = run({});

  assert.ok(classes.has('pending'));
  assert.equal(later.length, 1);
  assert.ok(later[0].ms > 0 && later[0].ms <= 2000);

  later[0].fn();
  assert.ok(!classes.has('pending'));
});

test('the theme of the settings is put on by the path the server versions', () => {
  const html = written({ 'freenet.interface': JSON.stringify({ theme: 'dark' }) });

  assert.equal(html, '<link id="theme" rel="stylesheet" href="/static/themes/dark.css">');
});

test('the theme of the device wins over the one of the settings', () => {
  const html = written({ 'freenet.theme': 'blue', 'freenet.interface': JSON.stringify({ theme: 'dark' }) });

  assert.match(html, /themes\/blue\.css/);
});

test('nothing is written for the default theme, an unknown one or broken storage', () => {
  assert.equal(written({}), '');
  assert.equal(written({ 'freenet.interface': JSON.stringify({ theme: 'default' }) }), '');
  assert.equal(written({ 'freenet.theme': '"><script src=//evil>' }), '');
  assert.equal(written({ 'freenet.theme': 'constructor' }), '');
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
