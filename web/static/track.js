// What the lines of the players and their keys mean, without the page:
// the big player (bigplayer.js) and the rows of songs (audio.js).

// trackAt is the part of a line under the pointer: 0 at its left end, 1
// at its right one, whatever side of the line the pointer has gone to.
export function trackAt(x, left, width) {
  if (!(width > 0)) return 0;

  return Math.max(0, Math.min(1, (x - left) / width));
}

// The keys of openvk's big player (al_music.js): space plays and pauses,
// the arrows seek three seconds, W/A and S/D go to the song before and
// after, R repeats, M mutes, + and - of the keypad change the loudness.
// The code is of the key itself, so they work in any layout.
const SHORTCUTS = {
  Space: ['toggle'],
  ArrowLeft: ['skip', -3],
  ArrowRight: ['skip', 3],
  KeyW: ['prev'],
  KeyA: ['prev'],
  KeyS: ['next'],
  KeyD: ['next'],
  KeyR: ['repeat'],
  KeyM: ['mute'],
  NumpadAdd: ['louder', 10],
  NumpadSubtract: ['louder', -10],
};

// shortcut is what a key does to the player: [command, value], or null.
export function shortcut(code) {
  return SHORTCUTS[code] ?? null;
}
