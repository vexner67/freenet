import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const read = (path) => readFileSync(new URL(`../static/${path}`, import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');

// The palette of a stylesheet: the variables its :root gives.
function palette(css) {
  const root = /:root\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
  return new Map([...root.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
}

const style = read('style.css');
const plain = palette(style);

test('the stylesheet writes colours only as variables', () => {
  const rules = style.replace(/:root\s*\{[^}]*\}/, '');
  const written = rules.match(/#[0-9a-f]{3,8}\b|(?<![-\w])(?:white|black|gr[ae]y|red|silver)(?![-\w(])/gi) ?? [];

  assert.deepEqual(written, [], 'a colour of style.css is a variable of its :root, so that a theme can change it');
});

test('every variable the stylesheet uses is in its palette', () => {
  const used = new Set([...style.matchAll(/var\((--[\w-]+)\)/g)].map((m) => m[1]));

  assert.deepEqual([...used].filter((name) => !plain.has(name)), []);
  assert.deepEqual([...plain.keys()].filter((name) => !used.has(name)), [], 'a variable nothing uses');
});

test('a variable is named by its value in the plain theme', () => {
  for (const [name, value] of plain) {
    assert.match(name, /^--(back|text|line)-[0-9a-f]{3,6}$/);
    assert.equal(value, `#${name.split('-').pop()}`, name);
  }
});

test('the dark theme gives every variable of the palette', () => {
  const dark = palette(read('themes/dark.css'));

  assert.deepEqual([...plain.keys()].filter((name) => !dark.has(name)), [], 'a colour the dark theme leaves light');
  assert.deepEqual([...dark.keys()].filter((name) => !plain.has(name)), [], 'a variable the stylesheet does not have');
});

// The admin panel has its own palette: AUI's, light under html.aui and
// dark under html.aui.aui-theme-dark — on the page, not on every .aui, or
// a form.aui would turn light again in the dark mode.
test('the admin panel writes colours only in its two palettes', () => {
  const css = read('admin.css');
  const block = (selector) => new RegExp(`^${selector}\\s*\\{([^}]*)\\}`, 'm').exec(css)?.[1] ?? '';
  const names = (body) => [...body.matchAll(/(--aui-[\w-]+)\s*:/g)].map((m) => m[1]);
  const light = block('html\\.aui');
  const dark = block('html\\.aui\\.aui-theme-dark');

  const rules = css.replace(/^html\.aui(\.aui-theme-dark)?\s*\{[^}]*\}/gm, '');
  assert.deepEqual(rules.match(/#[0-9a-f]{3,8}\b|(?<![-\w])(?:white|black|gr[ae]y|red|silver)(?![-\w(])/gi) ?? [], []);
  assert.deepEqual(names(dark), names(light), 'the dark palette gives every colour of the light one');

  const used = new Set([...rules.matchAll(/var\((--aui-[\w-]+)\)/g)].map((m) => m[1]));
  assert.deepEqual([...used].filter((name) => !names(light).includes(name)), []);
  assert.deepEqual(names(light).filter((name) => !used.has(name)), [], 'a colour nothing uses');
});
