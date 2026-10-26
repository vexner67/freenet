// The recipient field of the bank's "Перевести" and of "Подарить подписку"
// of Prime: the recipient is named by the address of their page, or found
// by name among the people of the site, or picked from those you follow —
// whoever is picked is shown under the field.

import { api } from './api.js';
import { transferNickname } from './labels.js';
import { avatarURL, el, fullName, pagePath, verifiedMark } from './layout.js';
import { tr } from './i18n.js';

// isNickname says whether a word could be the address of a page.
export function isNickname(word) {
  return /^[A-Za-z](?:[A-Za-z0-9_]*[A-Za-z0-9])?$/.test(word);
}

// recipientPicker drives the field to (with the list people and the line
// shown under it) for me; picked is called with whoever is chosen from the
// list. It returns found, the one picked or looked up (null when none),
// lookUp, which looks up what is typed, and clear.
export function recipientPicker({ to, people, shown, me, picked = () => {} }) {
  let found = null;
  function show(recipient) {
    found = recipient;
    shown.replaceChildren(el('a', { href: pagePath(recipient), textContent: fullName(recipient) }), verifiedMark(recipient));
    shown.hidden = false;
  }

  async function lookUp() {
    const nickname = transferNickname(to.value);
    found = null;
    shown.hidden = true;
    if (!isNickname(nickname)) return null;

    try {
      const recipient = await api.profile(nickname);
      if (transferNickname(to.value) !== nickname) return null;
      show(recipient);
    } catch {
      shown.replaceChildren(tr('Получатель не найден.'));
      shown.hidden = false;
    }

    return found;
  }

  // The people to pick from: those you follow while the field is empty,
  // those whose name or nickname has the words otherwise.
  let asked = 0;
  async function suggest() {
    const ask = ++asked;
    const text = to.value.trim();
    let list = [];
    let title = '';
    try {
      if (!text) {
        list = (await api.following(me.nickname, '', 20)).users ?? [];
        title = tr('Ваши подписки');
      } else if (!/[/?]/.test(text)) {
        list = (await api.search(text.replace(/^@/, ''))).users ?? [];
        title = tr('Найденные люди');
      }
    } catch {
      list = [];
    }
    if (ask !== asked) return;

    list = list.filter((u) => u.nickname && u.nickname !== me.nickname && !u.deactivated).slice(0, 8);
    people.replaceChildren(...(list.length ? [el('div', { className: 'bank-people-title', textContent: title }),
      ...list.map((person) => el('a', {
        className: 'bank-person',
        href: pagePath(person),
        onmousedown: (event) => event.preventDefault(),
        onclick: (event) => {
          event.preventDefault();
          to.value = person.nickname;
          people.hidden = true;
          show(person);
          picked(person);
        },
      }, el('img', { src: avatarURL(person, 'small'), alt: '', width: 30, height: 30 }),
      el('span', {}, el('b', { textContent: fullName(person) }), verifiedMark(person), el('br'),
        el('span', { className: 'hint', textContent: '@' + person.nickname }))))] : []));
    people.hidden = list.length === 0 || document.activeElement !== to;
  }

  let typing = 0;
  to.addEventListener('input', () => {
    found = null;
    shown.hidden = true;
    clearTimeout(typing);
    typing = setTimeout(suggest, 250);
  });
  to.addEventListener('focus', suggest);
  to.addEventListener('blur', () => { people.hidden = true; });
  to.addEventListener('change', lookUp);

  return {
    found: () => found,
    lookUp,
    clear() {
      to.value = '';
      found = null;
      shown.hidden = true;
    },
  };
}
