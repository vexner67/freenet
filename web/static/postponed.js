// Postponed posts, VK's "отложенные записи" (openvk has none): "Таймер"
// under the form of a wall, which keeps the post to come out at its time,
// the posts that wait on a tab of the wall page ("Отложенные записи") and
// the link to them above the wall, for its owner or a community's admins.

import { api } from './api.js';
import { inputDate, inputTime, localTime, nextHour, postponedCount, publishesAt } from './labels.js';
import { $, autosize, avatarURL, clubRef, el, fullName, hideError, pagePath, showError, textNode } from './layout.js';
import { button, openModal } from './modal.js';
import { photosNode, postPath } from './posts.js';

// postponedPath is the tab of the wall page with the posts that wait for
// the wall of owner, a person or a community.
export function postponedPath(owner) {
  const wall = owner.club
    ? '/club/' + encodeURIComponent(clubRef(owner)) + '/wall'
    : '/wall/' + encodeURIComponent(owner.nickname);

  return wall + '?type=postponed';
}

// timeInputs are a date and a time input showing iso on the visitor's
// clock; value() reads them back, '' unless both are filled in.
function timeInputs(iso) {
  const date = el('input', { type: 'date', value: inputDate(iso), ariaLabel: 'Дата публикации' });
  const time = el('input', { type: 'time', value: inputTime(iso), ariaLabel: 'Время публикации' });

  return { nodes: [date, ' в ', time], value: () => localTime(date.value, time.value) };
}

// initTimer is VK's "Таймер" of a form: the link opens the date and the
// time to publish at, the next full hour at first, and "×" takes them
// away; while they are there the button says "В очередь". onChange is
// told when the timer is set or taken away.
export function initTimer({ link, box, submit, onChange }) {
  const label = submit.textContent;
  let inputs = null;

  function set(on) {
    inputs = on ? timeInputs(nextHour()) : null;
    box.replaceChildren(...(inputs
      ? ['Запись будет опубликована ', ...inputs.nodes, ' ', el('a', { className: 'timer-remove', title: 'Не откладывать', textContent: '×', onclick: () => set(false) })]
      : []));
    box.hidden = !inputs;
    link.hidden = Boolean(inputs);
    submit.textContent = inputs ? 'В очередь' : label;
    onChange?.(Boolean(inputs));
  }

  link.hidden = false;
  link.addEventListener('click', () => set(true));

  return {
    active: () => Boolean(inputs),
    value: () => inputs?.value() ?? '',
    clear: () => set(false),
  };
}

// postponedNotice is the line a postponed post leaves above the wall
// instead of the post: when it comes out and the way to the others.
export function postponedNotice(p, owner) {
  return el('div', { className: 'post-deleted' },
    publishesAt(p.publishAt) + '. ',
    el('a', { href: postponedPath(owner), textContent: 'Отложенные записи' }),
  );
}

// showPostponedLink is the link above the wall of a page, VK's "N
// отложенных записей", for whoever may see them; none when nothing waits.
export async function showPostponedLink(owner) {
  if (!$('postponed')) return;

  try {
    const { postponed } = owner.club ? await api.clubPostponed(clubRef(owner)) : await api.myPostponed();

    $('postponed-link').textContent = postponedCount(postponed.length);
    $('postponed-link').href = postponedPath(owner);
    $('postponed').hidden = postponed.length === 0;
  } catch {
    // The page works without it.
  }
}

// wholeText loads all a postponed post says into it: the list carried
// its beginning.
async function wholeText(p) {
  if (p.textCut) {
    p.text = (await api.postponedPost(p.id)).text;
    p.textCut = false;
  }

  return p.text;
}

// showPostponedList shows the posts that wait for the wall of owner on
// the wall page, the soonest first: each with when it comes out,
// "Опубликовать сейчас", "Редактировать" and "Удалить".
export async function showPostponedList(owner) {
  const error = $('wall-error');
  let total = 0;

  function updateCount() {
    $('wall-count').textContent = postponedCount(total);
    $('wall-empty').textContent = 'Отложенных записей нет. Чтобы отложить запись, нажмите «Таймер» под формой новой записи.';
    $('wall-empty').hidden = total > 0;
  }

  function gone(node, line) {
    node.replaceWith(line);
    total--;
    updateCount();
  }

  async function publish(p, node) {
    hideError(error);

    try {
      const post = await api.publishPostponed(p.id);
      gone(node, el('div', { className: 'post-deleted' },
        'Запись опубликована. ',
        post ? el('a', { href: postPath(post), textContent: 'Перейти к ней' }) : ''));
    } catch (e) {
      showError(error, e);
    }
  }

  async function remove(p, node) {
    hideError(error);

    try {
      await api.deletePostponed(p.id);
      gone(node, el('div', { className: 'post-deleted', textContent: 'Запись удалена.' }));
    } catch (e) {
      showError(error, e);
    }
  }

  // edit is "Редактирование записи": the words, when it comes out and
  // whether it is adult; the photos stay. The list is read again after,
  // as the post may have moved.
  async function edit(p) {
    hideError(error);

    try {
      await wholeText(p);
    } catch (e) {
      showError(error, e);
      return;
    }

    const modal = openModal('Редактирование записи');
    const text = el('textarea', { className: 'publish-text', maxLength: 60000, value: p.text });
    const at = timeInputs(p.publishAt);
    const nsfw = el('input', { type: 'checkbox', checked: Boolean(p.nsfw) });
    const failed = el('div', { className: 'error', hidden: true });

    modal.body.append(
      text,
      el('div', { className: 'postponed-time' }, 'Запись будет опубликована ', ...at.nodes),
      el('label', { className: 'publish-signed' }, nsfw, ' Содержит NSFW-контент'),
      failed,
    );
    autosize(text);
    modal.actions.append(
      button('Сохранить', async (event) => {
        event.target.disabled = true;
        hideError(failed);

        try {
          await api.editPostponed(p.id, text.value, nsfw.checked, at.value());
          modal.close();
          await load();
        } catch (e) {
          showError(failed, e);
          event.target.disabled = false;
        }
      }),
      button('Отмена', modal.close, { secondary: true }),
    );
    text.focus();
  }

  // postponedNode is a post as it will come out: by the community when it
  // speaks for one, naming its author if signed; when instead of the date.
  function postponedNode(p) {
    const by = p.fromClub ? { ...owner, club: true } : p.author;
    const path = pagePath(by);
    const node = el('div', { className: 'post' },
      el('a', { className: 'post-avatar', href: path }, el('img', { src: avatarURL(by, 'small'), alt: '' })),
      el('div', { className: 'post-main' },
        el('div', { className: 'post-author' },
          el('a', { className: 'post-name', href: path, textContent: p.fromClub ? owner.name : fullName(p.author) }),
          el('div', { className: 'post-date', textContent: publishesAt(p.publishAt) + (p.nsfw ? ' · NSFW' : '') }),
        ),
        el('div', { className: 'post-content' },
          p.text ? textNode(p.text, { rest: p.textCut ? () => wholeText(p) : null }) : '',
          photosNode(p.photos) ?? '',
          p.signed ? el('div', { className: 'post-signature' }, el('span', { textContent: 'Автор: ' }), el('a', { href: pagePath(p.author), textContent: fullName(p.author) })) : '',
        ),
        el('div', { className: 'post-menu' },
          el('span', {},
            el('a', { textContent: 'Опубликовать сейчас', onclick: () => publish(p, node) }),
            ' | ',
            el('a', { textContent: 'Редактировать', onclick: () => edit(p) }),
            ' | ',
            el('a', { textContent: 'Удалить', onclick: () => remove(p, node) }),
          ),
        ),
      ),
    );

    return node;
  }

  async function load() {
    hideError(error);

    try {
      const { postponed } = owner.club ? await api.clubPostponed(clubRef(owner)) : await api.myPostponed();

      total = postponed.length;
      $('posts').replaceChildren(...postponed.map(postponedNode));
      updateCount();
    } catch (e) {
      showError(error, e);
    }
  }

  return load();
}
