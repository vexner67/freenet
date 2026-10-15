import test from 'node:test';
import assert from 'node:assert/strict';

import { storyAge, storyViewers, storyViews } from '../static/labels.js';

test('storyAge says how long ago a story was told', () => {
  const now = new Date('2026-10-16T12:00:00Z');
  const ago = (ms) => new Date(now - ms).toISOString();

  assert.equal(storyAge(ago(20_000), now), 'только что');
  assert.equal(storyAge(ago(60_000), now), '1 минуту назад');
  assert.equal(storyAge(ago(3 * 60_000), now), '3 минуты назад');
  assert.equal(storyAge(ago(25 * 60_000), now), '25 минут назад');
  assert.equal(storyAge(ago(60 * 60_000), now), '1 час назад');
  assert.equal(storyAge(ago(2 * 3_600_000 + 59 * 60_000), now), '2 часа назад');
  assert.equal(storyAge(ago(23 * 3_600_000), now), '23 часа назад');
});

test('the views of a story are counted in words', () => {
  assert.equal(storyViews(0), '0 просмотров');
  assert.equal(storyViews(1), '1 просмотр');
  assert.equal(storyViews(3), '3 просмотра');
  assert.equal(storyViews(11), '11 просмотров');
  assert.equal(storyViewers(1), 'Историю посмотрел 1 человек');
  assert.equal(storyViewers(5), 'Историю посмотрели 5 человек');
});
