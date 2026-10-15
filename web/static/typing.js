// "Печатает..." of a dialog or a chat, as VK has it (openvk has none):
// while you type, the page tells the others every few seconds, and what
// they tell shows under the messages until it is not told again for a
// while or their message comes.

import { typingText } from './labels.js';

// TELL_MS is how often a page tells that its user types; SHOWN_MS how long
// what others tell shows, a bit longer to bridge the pause between two.
const TELL_MS = 4000;
const SHOWN_MS = 6000;

// watchTyping calls tell while the user types in text, at most every
// TELL_MS; failures are let go, as the next keystroke tells again.
export function watchTyping(text, tell) {
  let told = 0;

  text.addEventListener('input', () => {
    const now = Date.now();
    if (!text.value.trim() || now - told < TELL_MS) return;

    told = now;
    tell().catch(() => {});
  });

  // A message sent ends the typing: the next keystroke tells at once.
  return { sent: () => { told = 0; } };
}

// typingLine keeps who is typing in one dialog or chat, calling render
// with what to say ('' for nobody) each time it changes: typing(user) when
// they tell it, stopped(id) when their message comes.
export function typingLine(render) {
  const typists = new Map();

  function changed() {
    render(typingText([...typists.values()].map(({ user }) => user)));
  }

  function stopped(id) {
    const typist = typists.get(id);
    if (!typist) return;

    clearTimeout(typist.timer);
    typists.delete(id);
    changed();
  }

  function typing(user) {
    clearTimeout(typists.get(user.id)?.timer);
    typists.set(user.id, { user, timer: setTimeout(() => stopped(user.id), SHOWN_MS) });
    changed();
  }

  return { typing, stopped };
}
