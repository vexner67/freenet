// Run: make web-test (node --test web/test/*.test.js)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { hashtagPath, textParts } from '../static/labels.js';

test('textParts finds the tags of a text, as post does', () => {
  assert.deepEqual(textParts('в поход'), ['в поход']);
  assert.deepEqual(textParts('#Поход завтра'), [{ hashtag: 'Поход', text: '#Поход' }, ' завтра']);
  assert.deepEqual(textParts('идём\n#лес, #summer_2026!'), [
    'идём\n', { hashtag: 'лес', text: '#лес' }, ', ', { hashtag: 'summer_2026', text: '#summer_2026' }, '!',
  ]);
  // Not inside a word, nor glued to another tag.
  assert.deepEqual(textParts('C# и url/#anchor'), ['C# и url/#anchor']);
  assert.deepEqual(textParts('#поход#лес'), [{ hashtag: 'поход', text: '#поход' }, '#лес']);
  assert.deepEqual(textParts('# и #'), ['# и #']);
  // A tag longer than 64 characters is no tag.
  const long = '#' + 'я'.repeat(65);
  assert.deepEqual(textParts(long), [long]);
});

test('textParts keeps the mentions, and no tag glued to one', () => {
  assert.deepEqual(textParts('[ann|Анна] #лес'), [{ nickname: 'ann', text: 'Анна' }, ' ', { hashtag: 'лес', text: '#лес' }]);
  assert.deepEqual(textParts('[ann|Анна]#лес'), [{ nickname: 'ann', text: 'Анна' }, '#лес']);
  assert.deepEqual(textParts('#лес @ann'), [{ hashtag: 'лес', text: '#лес' }, ' ', { nickname: 'ann', text: '@ann' }]);
});

test('hashtagPath leads to the search of posts', () => {
  assert.equal(hashtagPath('поход'), '/search?section=posts&q=%23%D0%BF%D0%BE%D1%85%D0%BE%D0%B4');
});
