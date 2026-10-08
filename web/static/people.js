// People in follow lists: a small cell for the profile's side blocks and
// a row for the full list page.

import { avatarURL, el, fullName } from './layout.js';

export function personCell(user) {
  const path = '/' + user.nickname;

  return el('div', { className: 'person-cell' },
    el('a', { href: path }, el('img', { src: avatarURL(user, 'small'), alt: '' })),
    el('a', { href: path, textContent: user.name }),
    el('div', { className: 'hint', textContent: user.surname }),
  );
}

// personRow is a person of a list; extra goes under the name, as the role
// and the links of a club's member.
export function personRow(user, ...extra) {
  const path = '/' + user.nickname;

  return el('div', { className: 'person' },
    el('a', { href: path }, el('img', { src: avatarURL(user, 'small'), alt: '' })),
    el('div', {},
      el('a', { className: 'post-name', href: path, textContent: fullName(user) }),
      user.status && el('div', { className: 'person-status', textContent: user.status }),
      ...extra,
    ),
  );
}
