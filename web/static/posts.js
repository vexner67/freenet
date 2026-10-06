// One post as on openvk: avatar, "Name wrote", date, text, and a menu with
// the link to the comments on the left and the like on the right.

import { api } from './api.js';
import { commentsLink, formatPostDate, wrote } from './labels.js';
import { avatarURL, el, fullName } from './layout.js';
import { openViewer } from './viewer.js';

const SVG = 'http://www.w3.org/2000/svg';

export function postPath(post) {
  return '/post/' + encodeURIComponent(post.id);
}

// postNode renders a post. Options:
//   onDelete(post, node) adds a "Удалить" link;
//   canLike makes the heart clickable (signed-in viewers);
//   onError(e) shows a failed like;
//   withCommentsLink: false on the page of the post, which shows the comments.
export function postNode(post, { onDelete, canLike = false, onError, withCommentsLink = true } = {}) {
  const path = '/' + post.author.nickname;

  const links = [];
  if (withCommentsLink) links.push(el('a', { href: postPath(post), textContent: commentsLink(post.comments) }));
  if (onDelete) links.push(el('a', { textContent: 'Удалить', onclick: () => onDelete(post, node) }));

  const node = el('div', { className: 'post' },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(post.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(post.author) }),
        ' ' + wrote(post.author.gender),
        el('div', { className: 'post-date' }, el('a', { href: postPath(post), textContent: formatPostDate(post.createdAt) })),
      ),
      post.text ? el('div', { className: 'post-text', textContent: post.text }) : null,
      photosNode(post.photos ?? []),
      el('div', { className: 'post-menu' },
        el('span', {}, ...separated(links)),
        likeButton(post, canLike, onError),
      ),
    ),
  );

  return node;
}

// photosNode lays a post's photos out: one at its own proportions, more as
// a grid of tiles. A click opens the viewer.
function photosNode(photos) {
  if (photos.length === 0) return null;

  // Rows stay full: 2 and 4 photos in two columns, 3 and more in three.
  const layout = photos.length === 1 ? 'single' : photos.length === 2 || photos.length === 4 ? 'grid-2' : 'grid-3';

  return el('div', { className: 'post-photos ' + layout }, ...photos.map((photo, i) =>
    el('a', {
      href: photo.large,
      onclick: (event) => {
        event.preventDefault();
        openViewer(photos, i);
      },
    }, el('img', { src: photo.small, alt: '', loading: 'lazy' }))));
}

// separated puts " | " between links, as in the menus of openvk.
export function separated(nodes) {
  return nodes.flatMap((node, i) => (i === 0 ? [node] : [el('span', { className: 'separator', textContent: '|' }), node]));
}

// likeButton is the heart with the number of likes. It changes post.likes
// and post.liked, so the post stays the source of truth.
function likeButton(post, canLike, onError) {
  const count = el('span', { className: 'like-count' });
  const button = el(canLike ? 'a' : 'span', { className: canLike ? 'like' : 'like disabled' }, heart(), count);

  function render() {
    button.classList.toggle('liked', post.liked);
    count.textContent = post.likes > 0 ? String(post.likes) : '';
    button.title = post.liked ? 'Вам нравится' : 'Нравится';
  }

  let busy = false;

  async function toggle() {
    if (busy) return;
    busy = true;

    try {
      const result = post.liked ? await api.unlike(post.id) : await api.like(post.id);
      post.likes = result.likes;
      post.liked = result.liked;
      render();
    } catch (e) {
      onError?.(e);
    } finally {
      busy = false;
    }
  }

  if (canLike) button.addEventListener('click', toggle);
  render();

  return button;
}

function heart() {
  const path = document.createElementNS(SVG, 'path');
  path.setAttribute('d', 'M5.5 9.6 1.3 5.4C.1 4.2.1 2.2 1.3 1 2.5-.2 4.4-.1 5.5 1.3 6.6-.1 8.5-.2 9.7 1c1.2 1.2 1.2 3.2 0 4.4Z');

  const svg = document.createElementNS(SVG, 'svg');
  svg.setAttribute('viewBox', '0 0 11 10');
  svg.setAttribute('aria-hidden', 'true');
  svg.append(path);

  return svg;
}
