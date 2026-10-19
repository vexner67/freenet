import assert from 'node:assert/strict';
import { test } from 'node:test';

import { nicknameLookup } from '../static/nicknames.js';

// lookup collects the requests made and runs them when told to.
function lookup(answer) {
  const asked = [];
  const timers = [];
  const known = nicknameLookup(async (nicknames) => {
    asked.push(nicknames);
    const known = answer(nicknames);
    return { nicknames: known, verified: known.filter((n) => n.toLowerCase() === 'vera') };
  }, (fn) => timers.push(fn));

  return { known, asked, run: () => timers.splice(0).forEach((fn) => fn()) };
}

test('the nicknames of a page are asked about together, each once', async () => {
  const { known, asked, run } = lookup((nicknames) => nicknames.filter((n) => n === 'ann' || n === 'vera').map((n) => n === 'ann' ? 'Ann' : 'Vera'));

  const answers = [known('Ann'), known('nobody'), known('ann'), known('vera')];
  assert.deepEqual(asked, []);
  run();

  const yes = { known: true, verified: false };
  const no = { known: false, verified: false };
  assert.deepEqual(await Promise.all(answers), [yes, no, yes, { known: true, verified: true }]);
  assert.deepEqual(asked, [['ann', 'nobody', 'vera']]);

  // What is known is not asked again.
  assert.deepEqual(await known('ANN'), yes);
  assert.deepEqual(await known('nobody'), no);
  run();
  assert.equal(asked.length, 1);
});

test('many nicknames go in requests of 50', async () => {
  const { known, asked, run } = lookup(() => []);

  const answers = Array.from({ length: 120 }, (_, i) => known('user' + i));
  run();

  assert.deepEqual(await Promise.all(answers), Array(120).fill({ known: false, verified: false }));
  assert.deepEqual(asked.map((a) => a.length), [50, 50, 20]);
});

test('a failed request is tried again when asked again', async () => {
  let fail = true;
  const { known, asked, run } = lookup((nicknames) => {
    if (fail) throw new Error('network');
    return nicknames;
  });

  const first = known('ann');
  run();
  await assert.rejects(first, /network/);

  fail = false;
  const second = known('ann');
  run();
  assert.equal((await second).known, true);
  assert.equal(asked.length, 2);
});
