// One post as on the old sites: avatar, "Name wrote", date, text, menu.

import { formatPostDate, wrote } from './labels.js';
import { avatarURL, el, fullName } from './layout.js';

// postNode renders a post; onDelete adds a "Удалить" link and is called
// with the post and its node.
export function postNode(post, onDelete) {
  const path = '/' + post.author.nickname;

  const node = el('div', { className: 'post' },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(post.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(post.author) }),
        ' ' + wrote(post.author.gender),
        el('div', { className: 'post-date', textContent: formatPostDate(post.createdAt) }),
      ),
      el('div', { className: 'post-text', textContent: post.text }),
      onDelete && el('div', { className: 'post-menu' },
        el('a', { textContent: 'Удалить', onclick: () => onDelete(post, node) })),
    ),
  );

  return node;
}
