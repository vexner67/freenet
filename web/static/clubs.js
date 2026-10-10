// A community in a list, as openvk's lists show it: the photo, a table
// with "Название" and "Размер", the description, and links on the right;
// an event ("встреча") also says when it starts and ends.

import { formatDateTime, membersCount } from './labels.js';
import { avatarURL, clubRef, el, pagePath } from './layout.js';

// clubRow renders club; actions are the links of the right column.
export function clubRow(club, ...actions) {
  const author = { ...club, club: true };
  const path = pagePath(author);

  return el('div', { className: 'list-row' },
    el('a', { className: 'list-cover', href: path }, el('img', { src: avatarURL(author, 'large'), alt: '' })),
    el('div', { className: 'list-info' },
      el('table', { className: 'info' },
        el('tr', {}, el('th', { textContent: 'Название:' }), el('td', {}, el('a', { href: path, textContent: club.name }))),
        el('tr', {}, el('th', { textContent: 'Размер:' }),
          el('td', {}, el('a', { href: `/club/${encodeURIComponent(clubRef(club))}/members`, textContent: membersCount(club.members) }))),
        club.startsAt && el('tr', {}, el('th', { textContent: 'Начало:' }), el('td', { textContent: formatDateTime(club.startsAt) })),
        club.endsAt && el('tr', {}, el('th', { textContent: 'Окончание:' }), el('td', { textContent: formatDateTime(club.endsAt) })),
      ),
      club.description && el('div', { className: 'list-description', textContent: club.description }),
    ),
    actions.length > 0 && el('div', { className: 'list-actions' }, ...actions),
  );
}
