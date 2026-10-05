// One comment as on openvk: a small avatar, the name, the text, then the
// date and the menu.

import { formatPostDate } from './labels.js';
import { avatarURL, el, fullName } from './layout.js';
import { separated } from './posts.js';

// commentNode renders a comment; onDelete adds a "Удалить" link and is
// called with the comment and its node.
export function commentNode(comment, onDelete) {
  const path = '/' + comment.author.nickname;

  const menu = [el('span', { className: 'post-date', textContent: formatPostDate(comment.createdAt) })];
  if (onDelete) menu.push(el('a', { textContent: 'Удалить', onclick: () => onDelete(comment, node) }));

  const node = el('div', { className: 'post comment' },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(comment.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(comment.author) }),
      ),
      el('div', { className: 'post-text', textContent: comment.text }),
      el('div', { className: 'post-menu' }, el('span', {}, ...separated(menu))),
    ),
  );

  return node;
}
