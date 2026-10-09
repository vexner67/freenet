// openvk's "Понравилось N людям": a tooltip over a heart with likes, the
// count and up to six photos, and from its head a box with everyone who
// likes the post or the comment, "Показать ещё" for more.

import { likedBy } from './labels.js';
import { avatarURL, el, fullName, pagePath, showError } from './layout.js';
import { button, openModal } from './modal.js';

const TOOLTIP_SHOWN = 6;
const BOX_PAGE = 24;
// As openvk's: shown after a pause, so a passing mouse does not open it,
// and kept a moment for the mouse to reach it.
const SHOW_DELAY = 400;
const HIDE_DELAY = 300;

// attachLikers shows the tooltip of item (its id and likes) over heart;
// load(id, cursor, limit) is the API call of its likers.
export function attachLikers(heart, item, load) {
  let tooltip = null;
  let showTimer = 0;
  let hideTimer = 0;

  function hide() {
    tooltip?.remove();
    tooltip = null;
  }

  async function show() {
    if (item.likes === 0) return;

    let page;
    try {
      page = await load(item.id, '', TOOLTIP_SHOWN);
    } catch {
      return; // A tooltip is a nicety: nothing to say if it fails.
    }

    hide();
    tooltip = el('div', { className: 'likers-tooltip' },
      el('a', {
        className: 'likers-tooltip-head',
        textContent: likedBy(page.total),
        onclick: () => {
          hide();
          openLikers(page.total, item.id, load);
        },
      }),
      el('div', { className: 'likers-tooltip-grid' }, ...page.users.map((user) => el('a', { href: pagePath(user), title: fullName(user) },
        el('img', { src: avatarURL(user, 'small'), alt: '' })))),
    );
    tooltip.addEventListener('mouseenter', () => clearTimeout(hideTimer));
    tooltip.addEventListener('mouseleave', leave);
    document.body.append(tooltip);

    // Above the heart, its right edge at the heart's, as openvk puts it.
    const rect = heart.getBoundingClientRect();
    tooltip.style.left = `${Math.max(4, rect.right + window.scrollX - tooltip.offsetWidth)}px`;
    tooltip.style.top = `${rect.top + window.scrollY - tooltip.offsetHeight - 4}px`;
  }

  function leave() {
    clearTimeout(showTimer);
    hideTimer = setTimeout(hide, HIDE_DELAY);
  }

  heart.addEventListener('mouseenter', () => {
    clearTimeout(hideTimer);
    if (!tooltip) showTimer = setTimeout(show, SHOW_DELAY);
  });
  heart.addEventListener('mouseleave', leave);
  // A new like or unlike changes who is there.
  heart.addEventListener('click', hide);
}

// openLikers is openvk's box of everyone who likes it, a page at a time.
export function openLikers(total, id, load) {
  const modal = openModal(likedBy(total));
  const grid = el('div', { className: 'likers-grid' });
  const error = el('div', { className: 'error', hidden: true });
  const more = el('a', { className: 'wall-more', textContent: 'Показать ещё', hidden: true });
  let cursor = '';

  async function loadMore() {
    more.hidden = true;

    try {
      const page = await load(id, cursor, BOX_PAGE);
      grid.append(...page.users.map((user) => el('div', { className: 'likers-item' },
        el('a', { href: pagePath(user) }, el('img', { src: avatarURL(user, 'small'), alt: '' })),
        el('a', { href: pagePath(user), textContent: fullName(user) }))));
      cursor = page.nextCursor;
      more.hidden = !cursor;
    } catch (e) {
      showError(error, e);
    }
  }

  more.addEventListener('click', loadMore);
  modal.body.append(error, grid, more);
  modal.actions.append(button('Закрыть', modal.close, { secondary: true }));
  modal.box.classList.add('likers-box');
  loadMore();
}
