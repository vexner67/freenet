// Stories, VK's "истории" (openvk has none): a photo with a few words shown
// for a day. Above the news is a row of circles, one per person with live
// stories, a ring around those not viewed yet; a click opens them over
// the whole screen, one after another, as VK plays them. Under each, in a
// white panel like that of the photo viewer, are the heart of a post and
// the comments, laid out as those under a post of a wall.

import { api } from './api.js';
import { storyAge, storyComments, storyViewers, storyViews } from './labels.js';
import { commentNode } from './comments.js';
import { autosize, avatarURL, el, fullName, pagePath, showError, verifiedMark } from './layout.js';
import { openLikers } from './likers.js';
import { button, openModal } from './modal.js';
import { likeButton, separated } from './posts.js';

// How long a story is shown before the next one, as on VK.
const STORY_SHOWN = 6000;
const MAX_TEXT = 200;
const MAX_COMMENT = 1000;
const COMMENTS_PAGE = 50;

// asked are the first pages of the comments of stories, by story, asked
// for as the story is shown: "Комментарии" then opens without waiting.
const asked = new Map();

function commentsOf(s) {
  if (!asked.has(s.id)) {
    const page = s.comments ? api.storyComments(s.id, '', COMMENTS_PAGE) : Promise.resolve({ comments: [], nextCursor: '' });
    page.catch(() => asked.delete(s.id));
    asked.set(s.id, page);
  }
  return asked.get(s.id);
}

// forget drops what was asked of a story whose comments changed.
function forget(s) {
  asked.delete(s.id);
}

// viewers is what the box of who viewed a story lists.
const viewers = {
  count: (story) => story.views,
  head: storyViewers,
  people: (page) => page.users.map((user) => ({ user, href: pagePath(user) })),
};

// initStoryBar fills bar with the stories above the news of me; one who
// filled in their profile can tell a story of their own.
// asked, when the page has asked already, are the stories first shown.
export async function initStoryBar(bar, me, asked = null) {
  let authors = [];

  function render() {
    const circles = authors.map((entry, i) => circle(entry.author, entry.viewed, fullName(entry.author), () => {
      openStories(authors, i, { onChange: render });
    }, verifiedMark(entry.author)));

    if (me.profileCompleted) {
      const add = circle(me, true, 'Добавить историю', () => openStoryForm(load));
      add.classList.add('story-add');
      circles.unshift(add);
    }

    bar.replaceChildren(...circles);
    bar.hidden = circles.length === 0;
  }

  async function load() {
    try {
      const got = asked ?? api.stories();
      asked = null;
      authors = (await got).authors;
    } catch {
      authors = []; // The stories are a nicety: the news go on without them.
    }
    render();
  }

  await load();
}

function circle(person, viewed, caption, onclick, mark = '') {
  return el('a', { className: viewed ? 'story-circle' : 'story-circle story-fresh', title: caption, onclick },
    el('img', { src: avatarURL(person, 'small'), alt: '' }),
    el('span', {}, caption, mark));
}

// openStories plays the stories of authors (as the API gives them) from
// the one at index: the first not viewed of each, or its first; onChange
// tells that some were viewed or deleted.
export function openStories(authors, index, { onChange = () => {} } = {}) {
  let author = index;
  let story = firstUnviewed(authors[author]);
  let started = 0;
  let elapsed = 0;
  let frame = 0;
  let ready = false;

  const progress = el('div', { className: 'story-progress' });
  const head = el('div', { className: 'story-head' });
  const photo = el('img', { className: 'story-photo', alt: '' });
  const text = el('div', { className: 'story-text' });
  const foot = el('div', { className: 'story-foot' });
  const comments = el('div', { className: 'story-comments', hidden: true });
  const stage = el('div', { className: 'story-stage' }, photo, text,
    el('a', { className: 'story-zone story-zone-prev', onclick: () => step(-1) }),
    el('a', { className: 'story-zone story-zone-next', onclick: () => step(1) }));
  const panel = el('div', { className: 'story-panel' }, foot, comments);
  const box = el('div', { className: 'story-box' }, progress, head, stage, panel);
  const overlay = el('div', { className: 'story-viewer' }, box,
    el('a', { className: 'viewer-nav viewer-prev', textContent: '‹', onclick: () => step(-1) }),
    el('a', { className: 'viewer-nav viewer-next', textContent: '›', onclick: () => step(1) }));

  function current() {
    return authors[author].stories[story];
  }

  // paused: a box over the stories (who viewed, deleting), the comments
  // or a tab in the background stops the time.
  function paused() {
    return !ready || document.hidden || !comments.hidden || document.querySelector('.modal') !== null;
  }

  function tick(now) {
    if (paused()) {
      started = now - elapsed;
    } else {
      elapsed = now - started;
    }

    const fill = progress.children[story]?.firstChild;
    if (fill) fill.style.width = `${Math.min(100, (elapsed / STORY_SHOWN) * 100)}%`;

    if (elapsed >= STORY_SHOWN) {
      step(1);
      return;
    }

    frame = requestAnimationFrame(tick);
  }

  function show() {
    const entry = authors[author];
    const s = current();

    cancelAnimationFrame(frame);
    elapsed = 0;
    ready = false;
    started = performance.now();

    progress.replaceChildren(...entry.stories.map((_, i) => el('div', { className: 'story-segment' },
      el('div', { className: i < story ? 'story-fill story-filled' : 'story-fill' }))));
    head.replaceChildren(
      el('a', { href: pagePath(entry.author) }, el('img', { src: avatarURL(entry.author, 'small'), alt: '' })),
      el('a', { href: pagePath(entry.author) }, fullName(entry.author), verifiedMark(entry.author)),
      el('span', { className: 'story-age', textContent: storyAge(s.createdAt) }),
      el('a', { className: 'story-close', textContent: '×', title: 'Закрыть', onclick: close }));
    text.textContent = s.text;
    text.hidden = !s.text;
    comments.hidden = true;
    comments.replaceChildren();
    renderFoot(s);
    commentsOf(s).catch(() => {}); // Shown when asked: a failure shows then.

    photo.onload = photo.onerror = () => {
      ready = true;
    };
    photo.src = s.photo.large;
    frame = requestAnimationFrame(tick);

    view(entry, s);
  }

  function view(entry, s) {
    if (s.viewed) return;

    s.viewed = true;
    entry.viewed = entry.stories.every((x) => x.viewed);
    api.viewStory(s.id).catch(() => {}); // Not counted this time: nothing to show for it.
    onChange();
  }

  function renderFoot(s) {
    // The heart of a post: the story has no list of who liked it.
    const heart = likeButton(s, {
      canLike: true,
      like: async (id) => {
        await api.likeStory(id);
        return { likes: s.likes + 1, liked: true };
      },
      unlike: async (id) => {
        await api.unlikeStory(id);
        return { likes: Math.max(0, s.likes - 1), liked: false };
      },
      likers: null,
    });
    const talk = el('a', { textContent: storyComments(s.comments), onclick: () => toggleComments(s) });

    foot.replaceChildren(el('span', { className: 'story-links' }, ...separated([talk, ...(s.mine ? ownLinks(s) : [])])), heart);
  }

  function toggleComments(s) {
    if (!comments.hidden) {
      comments.hidden = true;
      return;
    }

    comments.hidden = false;
    showComments(s);
  }

  // showComments lays the comments out as under a post of a wall: the
  // comments of the site, oldest first, "Показать ещё", and the field to
  // answer in at the bottom. They were asked for with the story, so they
  // open at once.
  async function showComments(s) {
    const list = el('div', { className: 'post-comments-list' });
    const more = el('a', { className: 'story-comments-more', textContent: 'Показать ещё', hidden: true });
    const error = el('div', { className: 'error', hidden: true });
    const field = el('textarea', { className: 'post-comment-text', rows: 1, maxLength: MAX_COMMENT, placeholder: 'Комментировать...', ariaLabel: 'Ваш комментарий' });
    const send = el('button', { className: 'button', type: 'submit', textContent: 'Отправить' });
    const form = el('form', { className: 'post-comment-form' }, field, send);
    const empty = el('p', { className: 'story-comments-empty', textContent: 'Комментариев пока нет.', hidden: true });
    let cursor = '';

    const render = (page) => {
      list.append(...page.comments.map((c) => commentOf(s, c)));
      cursor = page.nextCursor;
      more.hidden = !cursor;
      empty.hidden = list.children.length > 0;
    };

    more.addEventListener('click', async () => {
      more.hidden = true;
      try {
        render(await api.storyComments(s.id, cursor, COMMENTS_PAGE));
      } catch (e) {
        showError(error, e);
        more.hidden = false;
      }
    });
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      if (!field.value.trim()) return;

      send.disabled = true;
      try {
        const c = await api.commentStory(s.id, field.value);
        list.append(commentOf(s, c));
        empty.hidden = true;
        field.value = '';
        autosize(field);
        s.comments++;
        forget(s);
        renderFoot(s);
        comments.scrollTop = comments.scrollHeight;
      } catch (e) {
        showError(error, e);
      } finally {
        send.disabled = false;
      }
    });

    comments.replaceChildren(el('div', { className: 'post-comments story-comment-list' }, list, more, empty), error, form);

    try {
      const page = await commentsOf(s);
      if (current() === s && !comments.hidden) render(page);
    } catch (e) {
      showError(error, e);
    }
  }

  // commentOf is a comment of a story as one under a post: no heart, the
  // date not a link; its author and the author of the story delete it.
  function commentOf(s, c) {
    const node = commentNode(c, {
      withLike: false,
      onDelete: (c.mine || s.mine) ? async (comment, drawn) => {
        try {
          await api.deleteStoryComment(comment.id);
        } catch {
          return; // Gone already, or not ours: it stays until the next look.
        }
        drawn.remove();
        s.comments = Math.max(0, s.comments - 1);
        forget(s);
        renderFoot(s);
      } : null,
    });

    node.querySelector('.post-date')?.removeAttribute('href');
    return node;
  }

  function ownLinks(s) {
    // Nobody to list until somebody viewed it.
    const views = s.views
      ? el('a', { textContent: storyViews(s.views), onclick: () => openLikers(s.views, s.id, api.storyViewers, viewers) })
      : el('span', { textContent: storyViews(0) });

    return [views, el('a', { textContent: 'Удалить', onclick: () => confirmDelete(s) })];
  }

  function confirmDelete(s) {
    const modal = openModal('Удалить историю');
    const error = el('div', { className: 'error', hidden: true });

    modal.body.append(el('p', { textContent: 'Вы уверены, что хотите удалить эту историю?' }), error);
    modal.actions.append(
      button('Удалить', async () => {
        try {
          await api.deleteStory(s.id);
        } catch (e) {
          showError(error, e);
          return;
        }

        modal.close();
        removeStory(s);
      }),
      button('Отмена', modal.close, { secondary: true }));
  }

  function removeStory(s) {
    const entry = authors[author];

    entry.stories.splice(entry.stories.indexOf(s), 1);
    onChange();

    if (entry.stories.length > 0) {
      story = Math.min(story, entry.stories.length - 1);
      show();
      return;
    }

    authors.splice(author, 1);
    onChange();
    if (author >= authors.length) {
      close();
      return;
    }

    story = firstUnviewed(authors[author]);
    show();
  }

  // step goes to the next story (1) or the one before (-1), across the
  // people; past the last one the stories close.
  function step(by) {
    const next = story + by;

    if (next >= 0 && next < authors[author].stories.length) {
      story = next;
    } else if (by > 0 && author + 1 < authors.length) {
      author++;
      story = firstUnviewed(authors[author]);
    } else if (by < 0 && author > 0) {
      author--;
      story = 0;
    } else if (by > 0) {
      close();
      return;
    } else {
      story = 0;
    }

    show();
  }

  function onKey(event) {
    // A box over the stories and the comment being written have the keys.
    if (document.querySelector('.modal') || event.target.closest?.('textarea, input')) return;

    if (event.key === 'Escape') close();
    if (event.key === 'ArrowRight') step(1);
    if (event.key === 'ArrowLeft') step(-1);
  }

  function close() {
    cancelAnimationFrame(frame);
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  }

  overlay.addEventListener('click', (event) => {
    if (event.target === overlay) close();
  });
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
  show();
}

function firstUnviewed(entry) {
  const i = entry.stories.findIndex((s) => !s.viewed);
  return i < 0 ? 0 : i;
}

// openStoryForm is the box of a new story: a photo, uploaded as soon as
// it is chosen (as the photos of a post), and a few words over it; onTold
// reads the stories again.
export function openStoryForm(onTold) {
  const modal = openModal('Новая история');
  const file = el('input', { type: 'file', accept: 'image/jpeg,image/png,image/gif,image/webp' });
  const preview = el('img', { className: 'story-preview', alt: '', hidden: true });
  const caption = el('textarea', { maxLength: MAX_TEXT, placeholder: 'Подпись', ariaLabel: 'Подпись' });
  const error = el('div', { className: 'error', hidden: true });
  let photo = null;

  const publish = button('Опубликовать', async () => {
    if (!photo) {
      showError(error, 'Выберите фотографию для истории.');
      return;
    }

    publish.disabled = true;
    try {
      await api.tellStory(photo.id, caption.value);
    } catch (e) {
      showError(error, e);
      publish.disabled = false;
      return;
    }

    modal.close();
    onTold();
  });

  file.addEventListener('change', async () => {
    if (!file.files[0]) return;

    error.hidden = true;
    photo = null;
    preview.hidden = true;
    publish.disabled = true;
    try {
      photo = await api.uploadPhoto(file.files[0]);
      preview.src = photo.small;
      preview.hidden = false;
    } catch (e) {
      showError(error, e);
    } finally {
      publish.disabled = false;
    }
  });

  modal.body.append(
    el('p', { className: 'hint', textContent: 'История — фотография с подписью, которую 24 часа видят ваши подписчики над новостями.' }),
    file, preview, caption, error);
  modal.box.classList.add('story-form');
  modal.actions.append(publish, button('Отмена', modal.close, { secondary: true }));
}
