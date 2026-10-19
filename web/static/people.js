// People in follow lists: a small cell for the profile's side blocks and
// a row for the full list page, which says who is on the site now.

import { avatarURL, el, fullName, verifiedMark } from './layout.js';
import { tr } from './i18n.js';

export function personCell(user) {
  const path = '/' + user.nickname;

  return el('div', { className: 'person-cell' },
    el('a', { href: path }, el('img', { src: avatarURL(user, 'small'), alt: '' })),
    // One link of both names, the surname under the name and smaller, as
    // openvk's .cl_name: a long one fits the cell.
    el('a', { className: 'person-name', href: path },
      el('span', {}, user.name, verifiedMark(user)),
      el('span', { className: 'person-surname', textContent: user.surname })),
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
      verifiedMark(user),
      user.online && el('span', { className: 'person-online', textContent: tr('Онлайн') }),
      user.status && el('div', { className: 'person-status', textContent: user.status }),
      ...extra,
    ),
  );
}
