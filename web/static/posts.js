// One post as on openvk: avatar, "Name wrote", date, text, photos, the
// shared post of a repost, and a menu with the links to the comments and
// "Поделиться" on the left and the like on the right.

import { api } from './api.js';
import { attachLikers } from './likers.js';
import { commentsLink, formatPostDate, genitiveName, sharedBy, shareLink, updatedPhoto, wrote } from './labels.js';
import { adult, avatarURL, el, fullName, isMicroblog, pagePath, textNode } from './layout.js';
import { editPost } from './postedit.js';
import { openShare } from './share.js';
import { openViewer } from './viewer.js';
import { lastComments } from './postcomments.js';
import { pollNode } from './poll.js';
import { documentsNode } from './docs.js';

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
//   onArchived(post, node) runs after the post is put away in "Архив
//     записей" or returned from it, by whoever owns the wall, who also
//     closes its comments;
//   canLike makes the heart and "Поделиться" clickable (signed-in viewers);
//   onError(e) shows a failed like, edit or pin;
//   withCommentsLink: false on the page of the post, which shows the comments;
//   onWallOf: the viewer (null for anonymous) when posts of several walls
//     are mixed, as in the news feed: someone else's wall is then named,
//     "написал на стене Петра Петрова", as openvk does there only.
export function postNode(post, { viewer = null, moderates = false, onDelete, onPinned, onArchived, canLike = false, onError, withCommentsLink = true, onWallOf, found = '' } = {}) {
  const path = pagePath(post.author);
  const date = el('div', { className: 'post-date' });
  const content = el('div', { className: 'post-content' });
  const menu = el('span', {});
  // openvk's "Микроблог": no "написал" after the name, the date down in
  // the menu, "Поделиться" an arrow next to the heart, marks by the name
  // for what the words of the menu do, and under a post of a wall its
  // newest comments.
  const microblog = isMicroblog();
  const actions = el('span', { className: 'post-actions' });
  const comments = microblog && withCommentsLink
    ? lastComments(post, {
      commenter: viewer ?? onWallOf ?? null,
      moderates,
      path: postPath(post),
      canLike,
      onError,
      onChanged: () => render(),
    })
    : null;

  function render() {
    date.replaceChildren(
      el('a', { href: postPath(post), textContent: formatPostDate(post.createdAt) }),
      post.editedAt ? el('span', { className: 'post-edited', title: 'Отредактировано', textContent: ' (ред.)' }) : '',
      post.pinned ? ', закреплено' : '',
      post.archived ? ', в архиве' : '',
      post.commentsClosed ? ', комментарии закрыты' : '',
    );
    content.replaceChildren(
      // Whole on the page of the post, which has no link to open it.
      post.text ? textNode(post.text, { whole: !withCommentsLink, rest: post.textCut ? () => wholeText(post) : null, found }) : '',
      photosNode(post.photos ?? []) ?? '',
      documentsNode(post.documents) ?? '',
      post.poll ? pollNode(post, { canVote: canLike, onError }) : '',
      signatureNode(post.signer) ?? '',
    );
    menu.replaceChildren(...separated(menuLinks()));
    actions.replaceChildren(...(microblog ? actionMarks() : []));
    comments?.render();
    node.classList.toggle('pinned', Boolean(post.pinned));
    // Marked or unmarked by an edit, it is blurred or shown again.
    node.classList.toggle('post-nsfw', Boolean(post.nsfw));
  }

  // ownsWall: the viewer pins the post, puts it away and closes its
  // comments.
  const ownsWall = () => moderates || Boolean(viewer && wallOwnerID(post) === viewer.id);

  // changes are what the viewer may do to the post, in the order of
  // openvk: words in the menu or, in "Микроблог", marks by the name.
  function changes() {
    const found = [];
    if (canEdit(post, viewer)) found.push({ text: 'Редактировать', mark: pencil, run: edit });
    if (onDelete) found.push({ text: 'Удалить', mark: cross, run: () => onDelete(post, node) });
    if (ownsWall()) {
      // An archived post is on no wall to be pinned on.
      if (!post.archived) found.push({ text: post.pinned ? 'Открепить' : 'Закрепить', mark: pin, on: post.pinned, run: togglePin });
      found.push({ text: post.archived ? 'Разархивировать' : 'Архивировать', mark: box, on: post.archived, run: toggleArchived });
    }
    return found;
  }

  function menuLinks() {
    const links = [];
    if (comments) {
      // The comments are under the post; with none, the field opens.
      if (post.comments === 0 && comments.canOpen()) links.push(el('a', { textContent: 'Комментировать', onclick: openComments }));
    } else if (withCommentsLink) {
      links.push(el('a', { href: postPath(post), textContent: commentsLink(post.comments) }));
    }
    if (!microblog) {
      links.push(shareButton(post, canLike));
      links.push(...changes().map((c) => el('a', { textContent: c.text, onclick: c.run })));
    }
    if (ownsWall()) {
      links.push(el('a', { textContent: post.commentsClosed ? 'Открыть комментарии' : 'Закрыть комментарии', onclick: toggleComments }));
    }
    return links;
  }

  // actionMarks are openvk's marks of "Микроблог" by the name of the
  // author: each says what it does when pointed at.
  function actionMarks() {
    return changes().map((c) => el('a', { className: 'post-action' + (c.on ? ' on' : ''), title: c.text, onclick: c.run }, c.mark()));
  }

  function openComments() {
    comments.open();
    render();
  }

  // The form edits all the post says, not the beginning a list carried.
  async function edit() {
    if (node.classList.contains('editing')) return;

    try {
      await wholeText(post);
    } catch {
      return;
    }
    if (node.classList.contains('editing')) return;

    editPost(post, node, {
      onSaved: (edited) => {
        Object.assign(post, { textCut: false }, edited);
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

  // toggleArchived is openvk's "Архивировать" and "Разархивировать".
  async function toggleArchived() {
    if (pinning) return;
    pinning = true;

    try {
      if (post.archived) await api.unarchivePost(post.id);
      else await api.archivePost(post.id);
      post.archived = !post.archived;
      if (post.archived) post.pinned = false;
      render();
      onArchived?.(post, node);
    } catch (e) {
      onError?.(e);
    } finally {
      pinning = false;
    }
  }

  async function toggleComments() {
    if (pinning) return;
    pinning = true;

    try {
      if (post.commentsClosed) await api.openComments(post.id);
      else await api.closeComments(post.id);
      post.commentsClosed = !post.commentsClosed;
      render();
      node.dispatchEvent(new CustomEvent('commentsclosed', { detail: post.commentsClosed }));
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
        microblog && !post.avatarUpdate ? null : wroteBy(post),
        ...(onWallOf !== undefined && post.wall ? [' ', wallLink(post.wall, onWallOf)] : []),
        microblog ? actions : null,
        microblog ? null : date,
      ),
      content,
      post.repost ? sharedNode(post.repost, { canVote: canLike, onError }) : null,
      post.sharedPhoto ? sharedPhotoNode(post.sharedPhoto) : null,
      el('div', { className: 'post-menu' },
        el('span', {}, microblog ? date : null, menu),
        el('span', { className: 'post-buttons' },
          microblog ? shareButton(post, canLike, { icon: true }) : null,
          likeButton(post, { canLike, onError }),
        ),
      ),
      comments?.node ?? null,
    ),
  );

  revealOnClick(content);
  render();

  return node;
}

// revealOnClick shows a post marked as adult to whoever clicks its blurred
// content, as openvk does, and not to a guest. content is the blurred part
// of the post or of the shared post.
function revealOnClick(content) {
  content.addEventListener('click', (event) => {
    const marked = content.closest('.post-nsfw, .shared-nsfw');
    if (!marked || document.body.classList.contains('nsfw-shown')) return;

    event.preventDefault();
    event.stopPropagation();
    if (!document.body.classList.contains('guest')) marked.classList.remove('post-nsfw', 'shared-nsfw');
  }, true);
}

// canEdit: the author edits a post until editableUntil, a week after
// posting; the server checks it again. mine tells the author a post that
// speaks for a community, shown by the community.
export function canEdit(post, viewer, now = new Date()) {
  return Boolean(viewer) && Boolean(post.mine) && now < new Date(post.editableUntil);
}

// wroteBy is " написал" after the name of a person, " опубликовали"
// after a community's, as on openvk; for a new photo, openvk's "обновил
// фотографию на своей странице" or "обновило фотографию группы".
function wroteBy(post) {
  const { author } = post;
  if (post.avatarUpdate) return ' ' + updatedPhoto(author);

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
export function wallOwnerID(post) {
  return post.wall?.id ?? post.author.id;
}

// wholeText loads all a post says into it: a list carried its beginning.
async function wholeText(post) {
  if (post.textCut) {
    const whole = await api.post(post.id);
    post.text = whole.text;
    post.textCut = false;
  }

  return post.text;
}

// sharedNode is the post a repost shares, compact as on openvk: a small
// avatar, the name, the date, the text and photos, no menu. A post shared
// by the shared post itself is a link, not one more box inside. Its poll
// takes votes there too, of a signed-in reader (canVote).
export function sharedNode(shared, { canVote = false, onError } = {}) {
  if (shared.deleted) {
    return el('div', { className: 'post-shared deleted', textContent: 'Запись удалена.' });
  }

  const path = pagePath(shared.author);
  const content = el('div', { className: 'shared-content' },
    shared.text ? textNode(shared.text, { rest: shared.textCut ? () => wholeText(shared) : null }) : null,
    photosNode(shared.photos ?? []),
    documentsNode(shared.documents),
    shared.poll ? pollNode(shared, { canVote, onError }) : null,
  );
  revealOnClick(content);

  return el('div', { className: 'post-shared' + (shared.nsfw ? ' shared-nsfw' : '') },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(shared.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: 'post-name', href: path, textContent: fullName(shared.author) }),
        wroteBy(shared),
        ...(shared.wall ? [' ', wallLink(shared.wall, null)] : []),
        el('div', { className: 'post-date' }, el('a', { href: postPath(shared), textContent: formatPostDate(shared.createdAt) })),
      ),
      content,
      shared.repostOf ? el('a', { className: 'post-shared-link', href: postPath({ id: shared.repostOf }), textContent: 'Открыть запись' }) : null,
    ),
  );
}

// shareButton is "Поделиться (N)"; it changes post.reposts, as likeButton
// changes the likes.
// With icon it is the arrow and the number of openvk's "Микроблог".
function shareButton(post, canShare, { icon = false } = {}) {
  const count = el('span', { className: 'like-count' });
  const button = icon
    ? el(canShare ? 'a' : 'span', { className: 'like share-icon', title: 'Поделиться' }, arrow(), count)
    : el(canShare ? 'a' : 'span', {});

  function render() {
    if (icon) count.textContent = post.reposts > 0 ? String(post.reposts) : '';
    else button.textContent = shareLink(post.reposts);
  }

  if (canShare) {
    button.addEventListener('click', () => openShare(post, () => {
      post.reposts++;
      render();
    }));
  }
  // Who shared it, as who likes it over the heart: each leads to the
  // repost.
  attachLikers(button, post, api.reposts, sharers);
  render();

  return button;
}

const sharers = {
  count: (post) => post.reposts,
  head: sharedBy,
  people: (page) => page.posts.map((repost) => ({ user: repost.author, href: postPath(repost) })),
};

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
// sharedPhotoNode is the photo of an album a post shares, leading to its
// page, as openvk attaches a photo; a deleted one says so.
export function sharedPhotoNode(shared) {
  if (shared.deleted) {
    return el('div', { className: 'post-shared deleted', textContent: 'Фотография удалена.' });
  }

  return el('div', { className: 'post-photos single' },
    el('a', { href: '/photo/' + encodeURIComponent(shared.id) }, adult(el('img', { src: shared.photo.small, alt: '', loading: 'lazy' }), shared.nsfw)));
}

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
// comment with its own like, unlike and likers; over it, who like it
// (likers.js). It changes item.likes and
// item.liked, so the item stays the source of truth.
export function likeButton(item, { canLike = false, onError, like = api.like, unlike = api.unlike, likers = api.postLikers } = {}) {
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
  attachLikers(button, item, likers);
  render();

  return button;
}

function heart() {
  return icon('M5.5 9.6 1.3 5.4C.1 4.2.1 2.2 1.3 1 2.5-.2 4.4-.1 5.5 1.3 6.6-.1 8.5-.2 9.7 1c1.2 1.2 1.2 3.2 0 4.4Z');
}

// arrow is the mark of a repost: an arrow turning to the right.
function arrow() {
  return icon('M6.5 0 11 4 6.5 8V5.5C3.5 5.5 1.5 6.6 0 9.5 0 5.5 2.5 2.7 6.5 2.5Z');
}

// The marks of "Микроблог": a pencil edits, a cross deletes, a pin pins
// and a box puts away in the archive.
function pencil() {
  return icon('M0 10 .6 7.2 7.6.2a.7.7 0 0 1 1 0l1.2 1.2a.7.7 0 0 1 0 1L2.8 9.4Z');
}

function cross() {
  return icon('M1.5 1 5.5 4 9.5 1 10.5 2 6.8 5 10.5 8 9.5 9 5.5 6 1.5 9 .5 8 4.2 5 .5 2Z');
}

function pin() {
  return icon('M3.5 0h4L7 1.2V4l2 2v1H6l-.5 3L5 7H2V6l2-2V1.2Z');
}

function box() {
  return icon('M.5 1h10v2.5H.5ZM1.3 4.2h8.4V10H1.3ZM4 5.5v1h3v-1Z');
}

function icon(d) {
  const path = document.createElementNS(SVG, 'path');
  path.setAttribute('d', d);
  path.setAttribute('fill-rule', 'evenodd');

  const svg = document.createElementNS(SVG, 'svg');
  svg.setAttribute('viewBox', '0 0 11 10');
  svg.setAttribute('aria-hidden', 'true');
  svg.append(path);

  return svg;
}
