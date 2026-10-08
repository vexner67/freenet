// What the pages of a club's discussions ("Обсуждения") share: their
// paths and the title "Кошки » Обсуждения » …" of openvk.

import { clubRef, el, pagePath } from './layout.js';

export function topicPath(topic) {
  return '/topic/' + encodeURIComponent(topic.id);
}

export function boardPath(club) {
  return '/club/' + encodeURIComponent(clubRef(club)) + '/board';
}

// boardTitle is "<club> » Обсуждения » <what>", the club and the
// discussions as links; club is an author (club: true) or a club.
export function boardTitle(club, what) {
  return [
    el('a', { href: pagePath({ ...club, club: true }), textContent: club.name }),
    ' » ',
    el('a', { href: boardPath(club), textContent: 'Обсуждения' }),
    ' » ' + what,
  ];
}
