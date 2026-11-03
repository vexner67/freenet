// The smileys of dialogs and chats, the site's own, as the glossy ones of
// ICQ and QIP: "Смайлы" under the form opens them, a click puts the code
// of one in the words of the message, where the page shows its picture.
// The code stays in the words, so a sealed message carries it as any
// other word.

import { SMILEYS } from './labels.js';
import { el, isPhone, smileyImage } from './layout.js';

// insertCode puts code in input where its caret is, apart from the words
// around it, and tells the field it changed.
export function insertCode(input, code) {
  const { value, selectionStart: from, selectionEnd: to } = input;
  const before = from > 0 && !/\s/.test(value[from - 1]) ? ' ' : '';
  const after = to < value.length && /\s/.test(value[to]) ? '' : ' ';

  input.setRangeText(before + code + after, from, to, 'end');
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

// initSmileys makes link open panel, the smileys under the form, and put
// the picked one in input; the panel stays open for more. rival is the
// panel of stickers: one of the two is open at a time.
export function initSmileys({ link, panel, input, rival }) {
  panel.replaceChildren(...SMILEYS.map((smiley) => el('a', {
    className: 'smiley-pick',
    title: `${smiley.title} ${smiley.codes[0]}`,
    onclick: () => {
      insertCode(input, smiley.codes[0]);
      // On a phone the keyboard would cover the panel.
      if (!isPhone()) input.focus();
    },
  }, smileyImage(smiley))));

  link.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    if (!panel.hidden && rival) rival.panel.hidden = true;
  });
  rival?.link.addEventListener('click', () => {
    panel.hidden = true;
  });
}
