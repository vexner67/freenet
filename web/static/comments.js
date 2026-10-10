// One comment as on openvk: a small avatar, the name, the text, then the
// date, "в ответ" for a reply and the menu on the left, the heart on the
// right.

import { api } from './api.js';
import { formatPostDate } from './labels.js';
import { avatarURL, el, fullName, pagePath, textNode } from './layout.js';
import { likeButton, photosNode, separated } from './posts.js';

// commentAnchor is the id of a comment's node, for links to it.
export function commentAnchor(id) {
  return 'comment-' + id;
}

// commentHearts are how the heart of a comment is put, taken off and
// counted; a message of a topic has its own (topicHearts).
const commentHearts = { like: api.likeComment, unlike: api.unlikeComment, likers: api.commentLikers };

// topicHearts: nobody lists who liked a message of a topic.
export const topicHearts = { like: api.likeTopicMessage, unlike: api.unlikeTopicMessage, likers: null };

// commentNode renders a comment. Options:
//   onDelete(comment, node) adds a "Удалить" link;
//   onReply(comment) adds "Ответить";
//   canLike makes the heart clickable (signed-in viewers), onError shows
//   a failed like;
//   withLike: false for what has no likes, the messages of a topic.
export function commentNode(comment, { onDelete, onReply, canLike = false, onError, withLike = true, hearts = commentHearts } = {}) {
  // A comment for a community shows the community (fromWall).
  const path = pagePath(comment.author);

  const menu = [el('a', { className: 'post-date', href: '#' + commentAnchor(comment.id), textContent: formatPostDate(comment.createdAt) })];
  if (comment.replyTo) menu.push(el('a', { href: '#' + commentAnchor(comment.replyTo), textContent: 'в ответ' }));
  if (onDelete) menu.push(el('a', { textContent: 'Удалить', onclick: () => onDelete(comment, node) }));
  if (onReply) menu.push(el('a', { textContent: 'Ответить', onclick: () => onReply(comment) }));

  const like = withLike ? likeButton(comment, { canLike, onError, ...hearts }) : '';

  const node = el('div', { className: 'post comment', id: commentAnchor(comment.id) },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(comment.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(comment.author) }),
      ),
      comment.text ? textNode(comment.text) : null,
      photosNode(comment.photos ?? []),
      el('div', { className: 'post-menu' }, el('span', {}, ...separated(menu)), like),
    ),
  );

  return node;
}
