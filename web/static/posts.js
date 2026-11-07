// One post as on openvk: avatar, "Name wrote", date, text, photos, the
// shared post of a repost, and a menu with the links to the comments and
// "Поделиться" on the left and the like on the right.

import { api } from './api.js';
import { attachLikers } from './likers.js';
import { documentsCount, errorText, formatPostDate, videoDuration, genitiveName, photosCount, primeNameClass, sharedBy, votesCount, wrote } from './labels.js';
import { adult, avatarURL, el, externalLink, fullName, isMicroblog, isPhone, pagePath, textNode, verifiedMark } from './layout.js';
import { editPost } from './postedit.js';
import { openShare } from './share.js';
import { openViewer } from './viewer.js';
import { lastComments } from './postcomments.js';
import { pollNode } from './poll.js';
import { button, openMenu, openModal, openSheet, toast } from './modal.js';
import { documentsNode } from './docs.js';
import { audiosNode } from './audio.js';
import { articleCardNode, articleIn } from './articles.js';
import { tr } from './i18n.js';
import { drawBlurhash } from './blurhash.js';

// nameClass is the class of the name of an author by a post, a comment or
// a message: coloured as a person with Prime dressed it.
export function nameClass(author) {
  return ['post-name', primeNameClass(author)].filter(Boolean).join(' ');
}

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
//     "написал на стене Петра Петрова", as openvk does there only;
//   pageOf: the owner of the page the wall is on: their own posts there
//     go without the pattern of their page, which is already behind them;
//   feed: the post is in the news, which is openvk's feed to the letter on a
//     computer: the old post ends in the menu of oldpost.latte — the words
//     of what the viewer may do, "Комментарии (N)", "Поделиться (N)" and
//     the heart on the right — and "Микроблог" has the menu and the marks of
//     microblogpost.latte alone, with the field "Написать".
export function postNode(post, { viewer = null, moderates = false, onDelete, onPinned, onArchived, canLike = false, onError, withCommentsLink = true, onWallOf, pageOf, found = '', feed = false } = {}) {
  const path = pagePath(post.author);
  const date = el('div', { className: 'post-date' });
  const content = el('div', { className: 'post-content' });
  const menu = el('span', {});
  // openvk's "Микроблог": no "написал" after the name, the date down in
  // the menu, "Поделиться" an arrow next to the heart, marks by the name
  // for what the words of the menu do, and under a post of a wall its
  // newest comments.
  const microblog = isMicroblog();
  const phone = isPhone();
  const oldMenu = feed && !phone && !microblog ? el('span', {}) : null;
  const actions = el('span', { className: 'post-actions' });
  // "закреплено" grey after the name, as openvk's "Микроблог" has it in
  // the news, instead of after the date.
  const pinnedMark = microblog && feed ? el('span', { className: 'post-pinned', textContent: tr('закреплено') }) : null;
  const comments = microblog && withCommentsLink
    ? lastComments(post, {
      commenter: viewer ?? onWallOf ?? null,
      moderates,
      path: postPath(post),
      canLike,
      onError,
      onChanged: () => render(),
      placeholder: feed ? tr('Написать') : undefined,
    })
    : null;

  function render() {
    date.replaceChildren(
      el('a', { href: postPath(post), textContent: formatPostDate(post.createdAt) }),
      post.editedAt ? el('span', { className: 'post-edited', title: tr('Отредактировано'), textContent: tr(' (ред.)') }) : '',
      post.pinned && !pinnedMark ? tr(', закреплено') : '',
      post.archived ? tr(', в архиве') : '',
      post.commentsClosed ? tr(', комментарии закрыты') : '',
    );
    const article = articleIn(post.text);
    content.replaceChildren(
      // Whole on the page of the post, which has no link to open it. A
      // post that is the link to an article alone shows the card alone.
      post.text && post.text.trim() !== article?.url ? textNode(post.text, { whole: !withCommentsLink, rest: post.textCut ? () => wholeText(post) : null, found }) : '',
      photosNode(post.photos ?? []) ?? '',
      article ? articleCardNode(article) : '',
      linkCardNode(post.link) ?? '',
      documentsNode(post.documents) ?? '',
      audiosNode(post.audios) ?? '',
      paidNode(post, { canBuy: canLike, onBought: render }) ?? '',
      post.poll ? pollNode(post, { canVote: canLike, onError }) : '',
      signatureNode(post.signer) ?? '',
    );
    if (microblog && !phone) menu.replaceChildren(...separated(menuLinks()));
    oldMenu?.replaceChildren(...separated(oldLinks()));
    actions.replaceChildren(...(microblog ? actionMarks() : []));
    comments?.render();
    node.classList.toggle('pinned', Boolean(post.pinned));
    if (pinnedMark) pinnedMark.hidden = !post.pinned;
    // Marked or unmarked by an edit, it is blurred or shown again.
    node.classList.toggle('post-nsfw', Boolean(post.nsfw));
  }

  // ownsWall: the viewer pins the post, puts it away and closes its
  // comments.
  const ownsWall = () => moderates || Boolean(viewer && wallOwnerID(post) === viewer.id);

  // changes are what the viewer may do to the post: in "Микроблог" marks
  // by the name, in the order of openvk's marks, otherwise words of the
  // menu in the order of its old posts.
  function changes() {
    const found = {};
    if (canEdit(post, viewer)) found.edit = { text: tr('Редактировать'), mark: pencil, run: edit };
    if (onDelete) found.delete = { text: tr('Удалить'), mark: cross, run: () => onDelete(post, node) };
    if (ownsWall()) {
      // An archived post is on no wall to be pinned on.
      if (!post.archived) found.pin = { text: post.pinned ? tr('Открепить') : tr('Закрепить'), mark: pin, on: post.pinned, run: togglePin };
      found.archive = { text: post.archived ? tr('Разархивировать') : tr('Архивировать'), mark: box, on: post.archived, run: toggleArchived };
    }
    // In the news the marks stand as openvk's, which float right from the
    // cross: the pencil, the box, the pin, the cross.
    let order = microblog ? ['edit', 'delete', 'pin', 'archive'] : ['edit', 'archive', 'delete', 'pin'];
    if (microblog && feed) order = ['edit', 'archive', 'pin', 'delete'];
    return order.map((name) => found[name]).filter(Boolean);
  }

  // menuLinks are the words under a post of "Микроблог": the comments
  // are under the post, and with none the field opens; closing them,
  // which openvk has not, goes next.
  function menuLinks() {
    // The news has openvk's "Комментировать" alone.
    const closing = ownsWall() && !feed
      ? el('a', { textContent: post.commentsClosed ? tr('Открыть комментарии') : tr('Закрыть комментарии'), onclick: toggleComments })
      : null;

    const links = [];
    if (comments && post.comments === 0 && comments.canOpen()) links.push(el('a', { textContent: tr('Комментировать'), onclick: openComments }));
    if (closing) links.push(closing);
    if (!feed) links.push(copyLink(post));
    return links;
  }

  // oldLinks are the words of oldpost.latte's menu in the news: what the
  // viewer may do to the post, in its order, then "Комментарии (N)" and
  // "Поделиться (N)".
  function oldLinks() {
    const links = changes().map((c) => el('a', { textContent: c.text, onclick: c.run }));
    if (withCommentsLink) links.push(commentsLink(post));
    links.push(share);
    return links;
  }

  // sheetItems are what the viewer may do to the post: the words of the
  // menu, in the list of "⋯".
  function sheetItems() {
    const items = changes().map((c) => ({ text: c.text, run: c.run, danger: c.mark === cross }));
    if (ownsWall()) items.push({ text: post.commentsClosed ? tr('Открыть комментарии') : tr('Закрыть комментарии'), run: toggleComments });
    items.push({ text: tr('Скопировать ссылку'), run: () => copyToast(post) });
    return items;
  }

  // bar is the bar under a post, as the apps of phones have it: the
  // heart, the comments and "Поделиться" with their numbers, and "⋯" for
  // the rest — a list over the bottom of a phone, under "⋯" on a
  // computer.
  function bar() {
    const more = el('a', { className: 'like post-more', title: tr('Действия') }, dots());
    more.addEventListener('click', () => (phone ? openSheet(sheetItems()) : openMenu(more, sheetItems())));

    return el('div', { className: 'post-menu post-bar' },
      likeButton(post, { canLike, onError }),
      withCommentsLink ? el('a', { className: 'like post-comments-link', href: postPath(post), title: tr('Комментарии') },
        bubble(), el('span', { className: 'like-count', textContent: post.comments > 0 ? String(post.comments) : '' })) : null,
      shareButton(post, canLike),
      more,
    );
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

  // "Поделиться (N)" of the old menu keeps its count between renders.
  const share = oldMenu ? shareButton(post, canLike, true) : null;

  const node = el('div', { className: 'post' },
    el('a', { className: 'post-avatar', href: path },
      el('img', { src: avatarURL(post.author, 'small'), alt: '' }),
      // openvk's "Онлайн" under the photo of a person who is on the site.
      !microblog && !post.author.club && post.author.online ? el('span', { className: 'post-online', textContent: tr('Онлайн') }) : null,
    ),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: nameClass(post.author), href: path, textContent: fullName(post.author) }),
        verifiedMark(post.author),
        microblog ? null : wroteBy(post),
        ...(onWallOf !== undefined && post.wall ? [' ', wallLink(post.wall, onWallOf)] : []),
        pinnedMark ? ' ' : null,
        pinnedMark,
        microblog ? actions : null,
        microblog ? null : date,
      ),
      content,
      post.repost ? sharedNode(post.repost, { canVote: canLike, onError }) : null,
      oldMenu ? el('div', { className: 'post-menu' }, oldMenu, likeButton(post, { canLike, onError })) : null,
      oldMenu ? null : phone || !microblog ? bar() : el('div', { className: 'post-menu' },
        el('span', {}, date, menu),
        el('span', { className: 'post-buttons' },
          shareButton(post, canLike),
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

// remotePostNode is a post of another server of the Fediverse, or of a
// channel of Telegram, in the news: its author and the date lead there,
// its pictures and the card of the page it links to come through the
// site, and it has a heart of the site — its server does not learn of it
// — with local comments and reposts. The warning its
// author put before it (summary) shows above the blurred text, as the
// servers of Mastodon show it. onError(e) shows a failed like. In the news
// (feed) a computer ends it as openvk's old post: "Комментарии (N)",
// "Поделиться" and the link to the original on the left, the heart on the
// right.
export function remotePostNode(post, { onError, withCommentsLink = true, shared = false, feed = false } = {}) {
  const remote = post.fediverse;
  const author = remote.author;
  const name = author.name || author.handle;
  const photos = remote.images.map((image) => ({ large: image.url, small: image.url }));
  const telegram = author.telegram;

  const content = el('div', { className: 'post-content' },
    post.text ? textNode(post.text, { whole: false, named: true }) : '',
    photosNode(photos) ?? '',
    videosNode(remote.videos ?? [], remote.url, telegram) ?? '',
    linkCardNode(post.link) ?? '',
  );

  const node = el('div', { className: (shared ? 'post-shared post-remote' : 'post post-remote') + (post.nsfw ? ' post-nsfw' : '') },
    el('a', { className: 'post-avatar', href: author.url, target: '_blank', rel: 'noopener noreferrer nofollow ugc' },
      el('img', { src: author.avatar || '/static/avatar.svg', alt: '' }),
    ),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        externalLink(author.url, [name]),
        ' ',
        el('span', { className: 'post-handle', textContent: author.handle }),
        el('div', { className: 'post-date' },
          el('a', { href: postPath(post), textContent: formatPostDate(post.createdAt) }),
          post.editedAt ? el('span', { className: 'post-edited', title: tr('Отредактировано'), textContent: tr(' (ред.)') }) : '',
        ),
      ),
      remote.summary ? el('div', { className: 'post-summary', textContent: remote.summary }) : null,
      content,
      shared ? null : feed && !isPhone() ? el('div', { className: 'post-menu' },
        el('span', {}, ...separated([
          withCommentsLink ? commentsLink(post) : null,
          remoteShareButton(post, true),
          externalLink(remote.url, [telegram ? tr('Открыть в Telegram') : tr('Открыть в Fediverse')]),
        ].filter(Boolean))),
        likeButton(post, { canLike: true, onError, like: api.likeFediverse, unlike: api.unlikeFediverse, likers: api.fediverseLikers }),
      ) : el('div', { className: 'post-menu post-bar' },
        likeButton(post, { canLike: true, onError, like: api.likeFediverse, unlike: api.unlikeFediverse, likers: api.fediverseLikers }),
        withCommentsLink ? el('a', { className: 'like post-comments-link', href: postPath(post), title: tr('Комментарии') },
          bubble(), el('span', { className: 'like-count', textContent: post.comments > 0 ? String(post.comments) : '' })) : null,
        remoteShareButton(post),
        el('span', { className: 'post-from', textContent: telegram ? tr('Из Telegram') : tr('Из Fediverse') }),
        externalLink(remote.url, [telegram ? tr('Открыть в Telegram') : tr('Открыть в Fediverse')]),
      ),
    ),
  );

  revealOnClick(content);

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
// after a community's, as on openvk.
function wroteBy(post) {
  const { author } = post;

  return ' ' + (author.club ? tr('опубликовали') : wrote(author.gender));
}

// signatureNode is openvk's "Автор: Иван Иванов" under a post that speaks
// for a community and names its author ("Подпись автора").
function signatureNode(signer) {
  if (!signer) return null;

  return el('div', { className: 'post-signature' },
    el('span', { textContent: tr('Автор: ') }),
    el('a', { href: pagePath(signer), textContent: fullName(signer) }),
    verifiedMark(signer),
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

// paidNode is what a paid post says of its photos and documents: to one
// who has not opened them, how many there are and "Открыть за N голосов"
// (canBuy: a signed-in viewer), which opens them and runs onBought; to the
// author, what others pay for them.
function paidNode(post, { canBuy, onBought }) {
  if (post.mine && post.price > 0) {
    return el('div', { className: 'post-paid-note', textContent: tr('Фото и документы этой записи другие открывают за {0}.', votesCount(post.price)) });
  }

  if (!post.locked) return null;

  const what = [
    post.lockedPhotos ? photosCount(post.lockedPhotos) : '',
    post.lockedDocuments ? documentsCount(post.lockedDocuments) : '',
  ].filter(Boolean).join(tr(' и '));
  const error = el('div', { className: 'error', hidden: true });
  const buy = el('button', { className: 'button', type: 'button', textContent: tr('Открыть за {0}', votesCount(post.price)) });

  buy.addEventListener('click', async () => {
    buy.disabled = true;
    error.hidden = true;

    try {
      Object.assign(post, (await api.buyPost(post.id)).post);
      onBought();
    } catch (e) {
      error.textContent = errorText(e);
      error.hidden = false;
      buy.disabled = false;
    }
  });

  return el('div', { className: 'post-paid' },
    el('div', { className: 'post-paid-title', textContent: tr('Скрыто автором: {0}.', what) }),
    canBuy ? buy : el('div', { className: 'hint', textContent: tr('Открыть их можно, войдя на сайт.') }),
    error,
  );
}

// sharedNode is the post a repost shares, compact as on openvk: a small
// avatar, the name, the date, the text and photos, no menu. A post shared
// by the shared post itself is a link, not one more box inside. Its poll
// takes votes there too, of a signed-in reader (canVote).
export function sharedNode(shared, { canVote = false, onError } = {}) {
  if (shared.deleted) {
    return el('div', { className: 'post-shared deleted', textContent: tr('Запись удалена.') });
  }

  if (shared.fediverse) return remotePostNode(shared, { shared: true, onError });

  const path = pagePath(shared.author);
  const content = el('div', { className: 'shared-content' });
  const fill = () => content.replaceChildren(
    shared.text ? textNode(shared.text, { rest: shared.textCut ? () => wholeText(shared) : null }) : '',
    photosNode(shared.photos ?? []) ?? '',
    linkCardNode(shared.link) ?? '',
    documentsNode(shared.documents) ?? '',
    audiosNode(shared.audios) ?? '',
    paidNode(shared, { canBuy: canVote, onBought: fill }) ?? '',
    shared.poll ? pollNode(shared, { canVote, onError }) : '',
  );
  fill();
  revealOnClick(content);

  return el('div', { className: ['post-shared', ...(shared.nsfw ? ['shared-nsfw'] : [])].join(' ') },
    el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(shared.author, 'small'), alt: '' })),
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        el('a', { className: nameClass(shared.author), href: path, textContent: fullName(shared.author) }),
        verifiedMark(shared.author),
        wroteBy(shared),
        ...(shared.wall ? [' ', wallLink(shared.wall, null)] : []),
        el('div', { className: 'post-date' }, el('a', { href: postPath(shared), textContent: formatPostDate(shared.createdAt) })),
      ),
      content,
      shared.repostOf ? el('a', { className: 'post-shared-link', href: postPath({ id: shared.repostOf }), textContent: tr('Открыть запись') }) : null,
    ),
  );
}

// copyLink puts the address of the post in the clipboard, as openvk's
// "Скопировать ссылку" of a sticker pack: anyone may open it, a guest
// only reads the post and its comments.
function copyLink(post) {
  const link = el('a', { className: 'post-copy-link', textContent: tr('Скопировать ссылку') });

  link.addEventListener('click', async () => {
    if (!await copyAddress(post)) return;

    link.textContent = tr('Ссылка скопирована!');
    setTimeout(() => { link.textContent = tr('Скопировать ссылку'); }, 2000);
  });

  return link;
}

// copyToast copies the address of the post on a phone and says so.
async function copyToast(post) {
  if (await copyAddress(post)) toast(tr('Ссылка скопирована!'));
}

// copyAddress puts the address of the post in the clipboard, or shows it
// to be copied by hand; it tells whether it was copied.
async function copyAddress(post) {
  const url = location.origin + postPath(post);
  if (await copied(url)) return true;

  showLink(url);
  return false;
}

// copied tells whether the text went to the clipboard: by the clipboard
// API, or by copying a selection where the browser keeps that shut.
async function copied(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const field = el('textarea', { className: 'copy-field', value: text, readOnly: true });
    document.body.append(field);
    field.select();
    try {
      return document.execCommand('copy');
    } catch {
      return false;
    } finally {
      field.remove();
    }
  }
}

// showLink gives the address to copy by hand where nothing copies.
function showLink(url) {
  const modal = openModal(tr('Ссылка на запись'));
  const field = el('input', { type: 'text', value: url, readOnly: true });

  modal.body.append(el('div', { className: 'invite-link' }, field));
  modal.actions.append(button(tr('Закрыть'), modal.close));
  field.select();
}

// shareButton is "Поделиться": the arrow and the number, as openvk's
// "Микроблог" has it, or the words "Поделиться (N)" of its old post
// (words); it changes post.reposts, as likeButton changes the likes.
function shareButton(post, canShare, words = false) {
  const count = el('span', { className: words ? '' : 'like-count' });
  const button = words
    ? el(canShare ? 'a' : 'span', { className: 'post-share-link' }, tr('Поделиться'), count)
    : el(canShare ? 'a' : 'span', { className: 'like share-icon', title: tr('Поделиться') }, arrow(), count);

  function render() {
    if (words) count.replaceChildren(...counted(post.reposts));
    else count.textContent = post.reposts > 0 ? String(post.reposts) : '';
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

// remoteShareButton shares the imported post by its Freenet ID, with
// the same destinations as local posts, without a local repost counter:
// the arrow, or the word of the old post (words).
function remoteShareButton(post, words = false) {
  const button = words
    ? el('a', { className: 'post-share-link', textContent: tr('Поделиться') })
    : el('a', { className: 'like share-icon', title: tr('Поделиться') }, arrow());
  button.addEventListener('click', () => openShare(post, () => {}));

  return button;
}

// commentsLink is openvk's "Комментарии (N)" of the old post, leading to
// the comments on the page of the post.
function commentsLink(post) {
  return el('a', { className: 'post-comments-words', href: postPath(post) + '#comments' }, tr('Комментарии'), ...counted(post.comments));
}

// counted is " (N)" with the number bold after the words of openvk's old
// menu, nothing for none.
function counted(n) {
  return n > 0 ? [' (', el('b', { textContent: String(n) }), ')'] : [];
}

const sharers = {
  count: (post) => post.reposts,
  head: sharedBy,
  people: (page) => page.posts.map((repost) => ({ user: repost.author, href: postPath(repost) })),
};

// wallLink names the wall a post is on: "на вашей стене" for the viewer's,
// "в Любители котов" for a community's: openvk's "в $1", the name as is.
function wallLink(wall, viewer) {
  let text = tr('на стене ') + genitiveName(wall);
  if (wall.club) text = tr('в ') + wall.name;
  else if (viewer?.id === wall.id) text = tr('на вашей стене');

  return el('a', { className: 'post-wall', href: pagePath(wall), textContent: text });
}

// linkCardNode is the card of the page a post links to, as the page told
// it when the post was written: its picture, title, site and description,
// all leading to it.
export function linkCardNode(link) {
  if (!link) return null;

  const card = externalLink(link.url, [
    link.photo ? el('img', { className: 'link-card-photo', src: link.photo.small, alt: '', loading: 'lazy' }) : null,
    el('span', { className: 'link-card-text' },
      el('b', { className: 'link-card-title', textContent: link.title }),
      el('span', { className: 'link-card-site', textContent: link.site }),
      link.description ? el('span', { className: 'link-card-description', textContent: link.description }) : null),
  ].filter(Boolean));
  card.className = 'link-card';

  return card;
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

// videosNode shows the videos of a post of another server as their
// covers, which open the post there: the site does not play them. Until
// the cover comes, or when there is none, its blurhash is drawn; the
// canvas keeps the shape of the video, so nothing moves when it comes.
function videosNode(videos, url, telegram) {
  if (videos.length === 0) return null;

  return el('div', { className: 'post-videos' }, ...videos.map((video) => {
    const wide = video.width > 0 && video.height > 0 ? video.width / video.height : 16 / 9;
    const blur = el('canvas', { className: 'post-video-blur', width: 32, height: Math.max(1, Math.min(64, Math.round(32 / wide))) });
    drawBlurhash(blur, video.blurhash);

    const cover = el('img', { className: 'post-video-cover', src: video.cover, alt: video.description ?? '', loading: 'lazy' });
    cover.addEventListener('load', () => cover.classList.add('loaded'));
    cover.addEventListener('error', () => cover.remove());

    const duration = videoDuration(video.duration);

    return el('a', { className: 'post-video', href: url, target: '_blank', rel: 'noopener noreferrer nofollow ugc', title: telegram ? tr('Смотреть в Telegram') : tr('Смотреть в Fediverse') },
      blur,
      cover,
      el('span', { className: 'post-video-play' }),
      duration ? el('span', { className: 'post-video-time', textContent: duration }) : '',
    );
  }));
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
    button.title = item.liked ? tr('Вам нравится') : tr('Нравится');
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

// bubble is the mark of the comments, dots of what else may be done: on
// the bar of a phone.
export function bubble() {
  return icon('M1 0h9a1 1 0 0 1 1 1v5.5a1 1 0 0 1-1 1H5L2.5 10V7.5H1a1 1 0 0 1-1-1V1a1 1 0 0 1 1-1Z');
}

// dots are "⋯", the mark of a list of actions.
export function dots() {
  return icon('M1.3 3.7a1.3 1.3 0 1 0 0 2.6 1.3 1.3 0 0 0 0-2.6Zm4.2 0a1.3 1.3 0 1 0 0 2.6 1.3 1.3 0 0 0 0-2.6Zm4.2 0a1.3 1.3 0 1 0 0 2.6 1.3 1.3 0 0 0 0-2.6Z');
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
