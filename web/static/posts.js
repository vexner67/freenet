// One post as on openvk: avatar, "Name wrote", date, text, photos, the
// shared post of a repost, and a menu with the links to the comments and
// "Поделиться" on the left and the like on the right.

import { api } from './api.js';
import { commentsLink, formatPostDate, genitiveName, shareLink, wrote } from './labels.js';
import { avatarURL, el, fullName, pagePath } from './layout.js';
import { editPost } from './postedit.js';
import { openShare } from './share.js';
import { openViewer } from './viewer.js';

const SVG = 'http://www.w3.org/2000/svg';

export function postPath(post) {
  return '/post/' + encodeURIComponent(post.id);
}

// postNode renders a post. Options:
//   viewer: the signed-in viewer, null for anonymous; the author edits the
//     post for a week ("Редактировать"), the owner of the wall pins it
//     ("Закрепить"), as on openvk;
//   moderates: the viewer keeps this wall clean, as the admins of a
//     community do theirs: they pin any post too;
//   onDelete(post, node) adds a "Удалить" link;
//   onPinned(post) runs after the post is pinned or unpinned;
//   canLike makes the heart and "Поделиться" clickable (signed-in viewers);
//   onError(e) shows a failed like, edit or pin;
//   withCommentsLink: false on the page of the post, which shows the comments;
//   onWallOf: the viewer (null for anonymous) when posts of several walls
//     are mixed, as in the news feed: someone else's wall is then named,
//     "написал на стене Петра Петрова", as openvk does there only.
export function postNode(post, { viewer = null, moderates = false, onDelete, onPinned, canLike = false, onError, withCommentsLink = true, onWallOf } = {}) {
  const path = pagePath(post.author);
  const date = el('div', { className: 'post-date' });
  const content = el('div', { className: 'post-content' });
  const menu = el('span', {});

  function render() {
    date.replaceChildren(
      el('a', { href: postPath(post), textContent: formatPostDate(post.createdAt) }),
      post.editedAt ? el('span', { className: 'post-edited', title: 'Отредактировано', textContent: ' (ред.)' }) : '',
      post.pinned ? ', закреплено' : '',
    );
    content.replaceChildren(
      post.text ? el('div', { className: 'post-text', textContent: post.text }) : '',
      photosNode(post.photos ?? []) ?? '',
      signatureNode(post.signer) ?? '',
    );
    menu.replaceChildren(...separated(menuLinks()));
    node.classList.toggle('pinned', Boolean(post.pinned));
  }

  function menuLinks() {
    const links = [];
    if (withCommentsLink) links.push(el('a', { href: postPath(post), textContent: commentsLink(post.comments) }));
    links.push(shareButton(post, canLike));
    if (canEdit(post, viewer)) links.push(el('a', { textContent: 'Редактировать', onclick: edit }));
    if (onDelete) links.push(el('a', { textContent: 'Удалить', onclick: () => onDelete(post, node) }));
    if (moderates || (viewer && wallOwnerID(post) === viewer.id)) {
      links.push(el('a', { textContent: post.pinned ? 'Открепить' : 'Закрепить', onclick: togglePin }));
    }
    return links;
  }

  function edit() {
    if (node.classList.contains('editing')) return;

    editPost(post, node, {
      onSaved: (edited) => {
        Object.assign(post, edited);
        render();
      },
    });
  }

  let pinning = false;

  async function togglePin() {
    if (pinning) return;
    pinning = true;

    try {
      if (post.pinned) await api.unpinPost(post.id);
      else await api.pinPost(post.id);
      post.pinned = !post.pinned;
      render();
      onPinned?.(post);
    } catch (e) {
      onError?.(e);
    } finally {
      pinning = false;
    }
  }

  const node = el('div', { className: 'post' },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(post.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(post.author) }),
        wroteBy(post.author),
        ...(onWallOf !== undefined && post.wall ? [' ', wallLink(post.wall, onWallOf)] : []),
        date,
      ),
      content,
      post.repost ? sharedNode(post.repost) : null,
      el('div', { className: 'post-menu' },
        menu,
        likeButton(post, { canLike, onError }),
      ),
    ),
  );

  render();

  return node;
}

// canEdit: the author edits a post until editableUntil, a week after
// posting; the server checks it again. mine tells the author a post that
// speaks for a community, shown by the community.
export function canEdit(post, viewer, now = new Date()) {
  return Boolean(viewer) && Boolean(post.mine) && now < new Date(post.editableUntil);
}

// wroteBy is " написал" after the name of a person, " опубликовали"
// after a community's, as on openvk.
function wroteBy(author) {
  return ' ' + (author.club ? 'опубликовали' : wrote(author.gender));
}

// signatureNode is openvk's "Автор: Иван Иванов" under a post that speaks
// for a community and names its author ("Подпись автора").
function signatureNode(signer) {
  if (!signer) return null;

  return el('div', { className: 'post-signature' },
    el('span', { textContent: 'Автор: ' }),
    el('a', { href: pagePath(signer), textContent: fullName(signer) }),
  );
}

// wallOwnerID is whose wall the post is on: its wall is null on the
// author's own.
function wallOwnerID(post) {
  return post.wall?.id ?? post.author.id;
}

// sharedNode is the post a repost shares, compact as on openvk: a small
// avatar, the name, the date, the text and photos, no menu. A post shared
// by the shared post itself is a link, not one more box inside.
function sharedNode(shared) {
  if (shared.deleted) {
    return el('div', { className: 'post-shared deleted', textContent: 'Запись удалена.' });
  }

  const path = pagePath(shared.author);

  return el('div', { className: 'post-shared' },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(shared.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(shared.author) }),
        wroteBy(shared.author),
        ...(shared.wall ? [' ', wallLink(shared.wall, null)] : []),
        el('div', { className: 'post-date' }, el('a', { href: postPath(shared), textContent: formatPostDate(shared.createdAt) })),
      ),
      shared.text ? el('div', { className: 'post-text', textContent: shared.text }) : null,
      photosNode(shared.photos ?? []),
      shared.repostOf ? el('a', { className: 'post-shared-link', href: postPath({ id: shared.repostOf }), textContent: 'Открыть запись' }) : null,
    ),
  );
}

// shareButton is "Поделиться (N)"; it changes post.reposts, as likeButton
// changes the likes.
function shareButton(post, canShare) {
  const button = el(canShare ? 'a' : 'span', { textContent: shareLink(post.reposts) });

  if (canShare) {
    button.addEventListener('click', () => openShare(post, () => {
      post.reposts++;
      button.textContent = shareLink(post.reposts);
    }));
  }

  return button;
}

// wallLink names the wall a post is on: "на вашей стене" for the viewer's,
// "в Любители котов" for a community's: openvk's "в $1", the name as is.
function wallLink(wall, viewer) {
  let text = 'на стене ' + genitiveName(wall);
  if (wall.club) text = 'в ' + wall.name;
  else if (viewer?.id === wall.id) text = 'на вашей стене';

  return el('a', { className: 'post-wall', href: pagePath(wall), textContent: text });
}

// photosNode lays the photos of a post, comment or message out: one at its
// own proportions, more as a grid of tiles. A click opens the viewer.
export function photosNode(photos) {
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

// likeButton is the heart with the number of likes of a post, or of a
// comment with its own like and unlike. It changes item.likes and
// item.liked, so the item stays the source of truth.
export function likeButton(item, { canLike = false, onError, like = api.like, unlike = api.unlike } = {}) {
  const count = el('span', { className: 'like-count' });
  const button = el(canLike ? 'a' : 'span', { className: canLike ? 'like' : 'like disabled' }, heart(), count);

  function render() {
    button.classList.toggle('liked', item.liked);
    count.textContent = item.likes > 0 ? String(item.likes) : '';
    button.title = item.liked ? 'Вам нравится' : 'Нравится';
  }

  let busy = false;

  async function toggle() {
    if (busy) return;
    busy = true;

    try {
      const result = item.liked ? await unlike(item.id) : await like(item.id);
      item.likes = result.likes;
      item.liked = result.liked;
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
