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
