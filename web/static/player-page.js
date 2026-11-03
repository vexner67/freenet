// The window of the player (/player): a page opens it at its first song
// and sends it songs over the BroadcastChannel of audio.js, so the music
// goes on while the pages change (player.js).

import { $ } from './layout.js';
import { createPlayer } from './player.js';

const player = createPlayer($('player'));

// The first command shows the player in place of the hint.
new BroadcastChannel('freenet-player').addEventListener('message', ({ data }) => {
  if (data?.type === 'play') $('empty').hidden = true;
});
player.ready();
