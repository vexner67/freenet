import test from 'node:test';
import assert from 'node:assert/strict';
import { playbackPosition, needsRoomSeek, roomClockOffset, roomYouTubeID } from '../static/room-sync.js';

test('playing screen follows the server clock, paused screen does not', () => {
  const updatedAt = '2026-11-10T10:00:00.000Z';
  const now = Date.parse(updatedAt);
  assert.equal(playbackPosition({ position: 15, playing: true, updatedAt }, now + 5000), 20);
  assert.equal(playbackPosition({ position: 15, playing: false, updatedAt }, now + 5000), 15);
  assert.equal(playbackPosition({ position: 15, playing: true, updatedAt }, now - 5000), 15);
  assert.equal(playbackPosition({ position: NaN, playing: true }, now), 0);
});
test('only drift greater than two seconds seeks', () => {
  assert.equal(needsRoomSeek(1, 3), false);
  assert.equal(needsRoomSeek(1, 3.001), true);
  assert.equal(needsRoomSeek(3.001, 1), true);
  assert.equal(needsRoomSeek(NaN, 1), false);
});
test('clock offset uses the middle of the round trip', () => {
  assert.equal(roomClockOffset('1970-01-01T00:00:02Z', 900, 1100), 1000);
  assert.equal(roomClockOffset('', 900, 1100), 0);
});
test('video links name YouTube alone', () => {
  for (const link of ['https://youtu.be/dQw4w9WgXcQ', 'https://www.youtube.com/watch?v=dQw4w9WgXcQ', 'https://m.youtube.com/shorts/dQw4w9WgXcQ']) assert.equal(roomYouTubeID(link), 'dQw4w9WgXcQ');
  for (const link of ['javascript:alert(1)', 'https://youtube.com.evil/watch?v=dQw4w9WgXcQ', 'https://example.com/watch?v=dQw4w9WgXcQ', 'https://youtu.be/invalid']) assert.equal(roomYouTubeID(link), '');
});


test('one participant advances playback, with a deterministic fallback', async () => {
  const { roomPlaybackLeader } = await import('../static/room-sync.js');
  assert.equal(roomPlaybackLeader(['b', 'a'], 'b', 'everyone'), 'b');
  assert.equal(roomPlaybackLeader(['b', 'a'], 'owner', 'everyone'), 'a');
  assert.equal(roomPlaybackLeader(['a'], 'owner', 'owner'), '');
  assert.equal(roomPlaybackLeader([], 'owner', 'everyone'), '');
});
