// A community in OpenVK's list: a photo, a facts table and actions on the right.

import { clubTypes, membersCount } from './labels.js';
import { avatarURL, clubRef, el, pagePath, verifiedMark } from './layout.js';
import { tr } from './i18n.js';

// clubRow is the group list; search only shows the name and size.
export function clubRow(club, { search = false } = {}, ...actions) {
  const author = { ...club, club: true };
  const path = pagePath(author);

  return el('div', { className: 'list-row club-list-row' },
    el('a', { className: 'list-cover', href: path }, el('img', { src: avatarURL(author, 'large'), alt: '', loading: 'lazy' })),
    el('div', { className: 'list-info' },
      !search && el('div', { className: 'club-list-name' },
        el('a', { href: path, textContent: club.name }), verifiedMark(club)),
      el('table', { className: 'info' },
        el('tr', {}, el('th', { textContent: tr('Название:') }), el('td', {}, el('a', { href: path, textContent: club.name }), verifiedMark(club))),
        el('tr', {}, el('th', { textContent: tr('Размер:') }),
          el('td', {}, el('a', { href: `/club/${encodeURIComponent(clubRef(club))}/members`, textContent: membersCount(club.members) }))),
        !search && el('tr', {}, el('th', { textContent: tr('Тип группы:') }),
          el('td', { className: 'club-list-type', textContent: clubTypes[club.closed ? 1 : 0][1] })),
      ),
    ),
    actions.length > 0 && el('div', { className: 'list-actions' }, ...actions),
  );
}
