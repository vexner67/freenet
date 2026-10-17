// Stories, VK's "истории" (openvk has none): a photo with a few words shown
// for a day. Above the news is a row of circles, one per person with live
// stories, a ring around those not viewed yet; a click opens them over
// the whole screen, one after another, as VK plays them.

import { api } from './api.js';
import { storyAge, storyViewers, storyViews } from './labels.js';
import { avatarURL, el, fullName, pagePath, showError } from './layout.js';
import { openLikers } from './likers.js';
import { button, openModal } from './modal.js';

// How long a story is shown before the next one, as on VK.
const STORY_SHOWN = 6000;
const MAX_TEXT = 200;

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
    }));

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

function circle(person, viewed, caption, onclick) {
  return el('a', { className: viewed ? 'story-circle' : 'story-circle story-fresh', title: caption, onclick },
    el('img', { src: avatarURL(person, 'small'), alt: '' }),
    el('span', { textContent: caption }));
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
  const stage = el('div', { className: 'story-stage' }, photo, text,
    el('a', { className: 'story-zone story-zone-prev', onclick: () => step(-1) }),
    el('a', { className: 'story-zone story-zone-next', onclick: () => step(1) }));
  const box = el('div', { className: 'story-box' }, progress, head, stage, foot);
  const overlay = el('div', { className: 'story-viewer' }, box,
    el('a', { className: 'viewer-nav viewer-prev', textContent: '‹', onclick: () => step(-1) }),
    el('a', { className: 'viewer-nav viewer-next', textContent: '›', onclick: () => step(1) }));

  function current() {
    return authors[author].stories[story];
  }

  // paused: a box over the stories (who viewed, deleting) or a tab in
  // the background stops the time.
  function paused() {
    return !ready || document.hidden || document.querySelector('.modal') !== null;
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
      el('a', { href: pagePath(entry.author), textContent: fullName(entry.author) }),
      el('span', { className: 'story-age', textContent: storyAge(s.createdAt) }),
      el('a', { className: 'story-close', textContent: '×', title: 'Закрыть', onclick: close }));
    text.textContent = s.text;
    text.hidden = !s.text;
    foot.replaceChildren(...(s.mine ? ownLinks(s) : []));
    foot.hidden = !s.mine;

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
    // A box over the stories has the keys.
    if (document.querySelector('.modal')) return;

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
