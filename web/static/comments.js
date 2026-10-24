// One comment as on openvk: a small avatar, the name, the text, then the
// date, "в ответ" for a reply and the menu on the left, the heart on the
// right.

import { api } from './api.js';
import { formatPostDate, primeDress } from './labels.js';
import { avatarURL, el, fullName, pagePath, textNode, verifiedMark } from './layout.js';
import { likeButton, nameClass, photosNode, separated } from './posts.js';
import { tr } from './i18n.js';

// commentAnchor is the id of a comment's node, for links to it.
export function commentAnchor(id) {
  return 'comment-' + id;
}

// commentHearts are how the heart of a comment is put, taken off and
// counted; a message of a topic and a comment under a photo have their
// own (topicHearts, photoCommentHearts).
const commentHearts = { like: api.likeComment, unlike: api.unlikeComment, likers: api.commentLikers };

export const topicHearts = { like: api.likeTopicMessage, unlike: api.unlikeTopicMessage, likers: api.topicMessageLikers };

export const photoCommentHearts = { like: api.likePhotoComment, unlike: api.unlikePhotoComment, likers: api.photoCommentLikers };

// commentNode renders a comment. Options:
//   onDelete(comment, node) adds a "Удалить" link;
//   onReply(comment) adds "Ответить";
//   canLike makes the heart clickable (signed-in viewers), onError shows
//   a failed like;
//   withLike: false for what has no likes, the messages of a topic;
//   whole(id) loads all it says when a list carried the beginning: a
//   comment's by default, a topic message's, a photo comment's;
//   href: the page the comment is on, when it is shown on another (under
//   a post of a wall): its date leads there.
export function commentNode(comment, { onDelete, onReply, canLike = false, onError, withLike = true, hearts = commentHearts, whole = api.commentText, href = '' } = {}) {
  // A comment for a community shows the community (fromWall).
  const path = pagePath(comment.author);

  const menu = [el('a', { className: 'post-date', href: href + '#' + commentAnchor(comment.id), textContent: formatPostDate(comment.createdAt) })];
  if (comment.replyTo) menu.push(el('a', { href: href + '#' + commentAnchor(comment.replyTo), textContent: tr('в ответ') }));
  if (onDelete) menu.push(el('a', { textContent: tr('Удалить'), onclick: () => onDelete(comment, node) }));
  if (onReply) menu.push(el('a', { textContent: tr('Ответить'), onclick: () => onReply(comment) }));

  const like = withLike ? likeButton(comment, { canLike, onError, ...hearts }) : '';

  // Shown on another page, it is no target of links there.
  const node = el('div', { className: ['post', 'comment', ...primeDress(comment.author)].join(' '), id: href ? '' : commentAnchor(comment.id) },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(comment.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: nameClass(comment.author), href: path, textContent: fullName(comment.author) }),
        verifiedMark(comment.author),
      ),
      comment.text ? textNode(comment.text, { rest: comment.textCut ? () => whole(comment.id).then((c) => c.text) : null }) : null,
      photosNode(comment.photos ?? []),
      el('div', { className: 'post-menu' }, el('span', {}, ...separated(menu)), like),
    ),
  );

  return node;
}
