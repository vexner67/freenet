import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';

const statics = new URL('../static/', import.meta.url);
// The two scripts that are not modules: the theme before the first paint
// and the service worker.
const classic = new Set(['theme.js', 'sw.js']);

// check is what node says of the script name, '' when it parses.
function check(name) {
  const args = classic.has(name) ? ['--check', '-'] : ['--input-type=module', '--check'];

  return new Promise((resolve) => {
    const child = execFile(process.execPath, args, (error, _, stderr) => resolve(error ? stderr || error.message : ''));
    child.stdin.end(readFileSync(new URL(name, statics)));
  });
}

// A script the browser cannot parse leaves its page blank, and no other
// test imports the scripts that touch the page (a second function of one
// name in chat.js did so).
test('every script of the site parses', async () => {
  const names = readdirSync(statics).filter((n) => n.endsWith('.js'));
  const said = await Promise.all(names.map(check));

  names.forEach((name, i) => assert.equal(said[i], '', name));
});

// The names a module exports: export function, const, class and lists.
function exported(source) {
  const names = new Set();

  for (const m of source.matchAll(/^export\s+(?:async\s+)?(?:function\*?|const|let|class)\s+([\w$]+)/gm)) names.add(m[1]);
  for (const m of source.matchAll(/^export\s*\{([^}]*)\}/gm)) {
    for (const part of m[1].split(',')) {
      const name = part.trim().split(/\s+as\s+/).pop();
      if (name) names.add(name);
    }
  }
  return names;
}

// A name imported that the module does not export also leaves the page
// blank (a lost conversationOption of forward.js did so).
test('every module exports what is imported from it', () => {
  const names = readdirSync(statics).filter((n) => n.endsWith('.js') && !classic.has(n));
  const exports = new Map(names.map((n) => [n, exported(readFileSync(new URL(n, statics), 'utf8'))]));

  for (const name of names) {
    const source = readFileSync(new URL(name, statics), 'utf8');

    for (const m of source.matchAll(/import\s*\{([^}]*)\}\s*from\s*'\.\/([\w-]+\.js)'/g)) {
      for (const part of m[1].split(',')) {
        const wanted = part.trim().split(/\s+as\s+/)[0];
        if (wanted) assert.ok(exports.get(m[2])?.has(wanted), `${name} imports ${wanted} from ${m[2]}`);
      }
    }
  }
});
