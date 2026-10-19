// openvk's "Настройки ленты": a box with two tabs. "Основное" is how
// many posts a page of the news holds, whether the ignored sources are
// shown and whether what people wrote on the walls of others is;
// "Игнорируемые источники" lists the people and the communities whose
// walls the news leave out, to stop ignoring them. The link that ignores
// a page ("Игнорировать пользователя") is here too.

import { api } from './api.js';
import { defaultFeedSettings, feedPageSizes, feedSettings, ignoreLabel } from './labels.js';
import { avatarURL, el, fullName, pagePath, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const KEY = 'freenet.feed';

// At most this many sources stop being ignored at once, as on openvk.
const MAX_UNIGNORED = 10;

// readFeedSettings is what this browser keeps; storage may be closed to
// the page, then the news are as by default.
export function readFeedSettings() {
  try {
    return feedSettings(localStorage.getItem(KEY));
  } catch {
    return { ...defaultFeedSettings };
  }
}

function keep(settings) {
  try {
    localStorage.setItem(KEY, JSON.stringify(settings));
  } catch {
    // The settings then last until the page is left.
  }
}

// openFeedSettings shows the box. showIgnored is whether the page shows
// the ignored sources now; onApply(settings, showIgnored) runs after
// "Применить", onUnignored after sources stopped being ignored.
export function openFeedSettings({ showIgnored, onApply, onUnignored }) {
  const modal = openModal(tr('Настройки ленты'));
  const content = el('div', { className: 'feed-settings' });
  const error = el('div', { className: 'error', hidden: true });
  const tabs = {
    main: el('a', { textContent: tr('Основное'), onclick: () => open('main') }),
    ignored: el('a', { textContent: tr('Игнорируемые источники'), onclick: () => open('ignored') }),
  };
  const close = button(tr('Закрыть'), modal.close, { secondary: true });

  modal.box.classList.add('feed-settings-box');
  modal.body.append(el('div', { className: 'tabs modal-tabs' }, tabs.main, tabs.ignored), error, content);

  function open(name) {
    hideError(error);
    for (const [tab, link] of Object.entries(tabs)) link.classList.toggle('active', tab === name);
    content.replaceChildren();
    modal.actions.replaceChildren(close);

    if (name === 'main') {
      showMain();
    } else {
      showIgnoredSources().catch((e) => showError(error, e));
    }
  }

  function showMain() {
    const settings = readFeedSettings();
    const posts = el('select', { id: 'feed-posts' }, ...feedPageSizes.map((n) => el('option', { value: String(n), textContent: String(n) })));
    const ignored = el('input', { type: 'checkbox', id: 'feed-show-ignored', checked: showIgnored });
    const alien = el('input', { type: 'checkbox', id: 'feed-alien', checked: settings.alien });

    posts.value = String(settings.posts);
    content.append(
      el('label', { className: 'feed-settings-row', htmlFor: 'feed-posts' }, el('span', { textContent: tr('Количество записей на странице') }), posts),
      el('label', { className: 'modal-option' }, ignored, tr('Показывать игнорируемые источники')),
      el('label', { className: 'modal-option' }, alien, tr('Показывать посты на чужих стенах')),
    );
    modal.actions.prepend(button(tr('Применить'), () => {
      const chosen = { posts: Number(posts.value), alien: alien.checked };

      keep(chosen);
      modal.close();
      onApply(chosen, ignored.checked);
    }));
  }

  async function showIgnoredSources() {
    const { sources } = await api.ignored();

    if (sources.length === 0) {
      content.append(el('div', { className: 'hint', textContent: tr('Игнорируемых источников нет.') }));
      return;
    }

    const list = el('div', { className: 'ignored-list' });
    const stop = button(tr('Не игнорировать'), async () => {
      hideError(error);
      stop.disabled = true;

      try {
        await api.unignore(checked().map((box) => box.value));
        onUnignored();
        open('ignored');
      } catch (e) {
        showError(error, e);
        stop.disabled = false;
      }
    });

    function checked() {
      return [...list.querySelectorAll('input:checked')];
    }

    stop.disabled = true;
    list.addEventListener('change', (event) => {
      // No more than one request takes.
      if (checked().length > MAX_UNIGNORED) event.target.checked = false;
      stop.disabled = checked().length === 0;
    });
    list.append(...sources.map(sourceRow));
    content.append(list);
    modal.actions.prepend(stop);
  }

  open('main');
}

// sourceRow is an ignored person or community: the photo, the name and
// the box that picks it.
function sourceRow(source) {
  const name = fullName(source) || source.nickname;

  return el('label', { className: 'ignored-row' },
    el('img', { src: avatarURL(source, 'small'), alt: '' }),
    el('a', { href: pagePath(source), textContent: source.deactivated ? tr('{0} (страница удалена)', name) : name }),
    el('input', { type: 'checkbox', value: source.id }),
  );
}

// initIgnoreLink wires the link of a page: kind is whose page it is
// ('user' or 'club'), change(ignore) asks the server.
export function initIgnoreLink(link, { kind, ignored, change, error }) {
  let busy = false;

  function render() {
    link.textContent = ignoreLabel(kind, ignored);
  }

  link.addEventListener('click', async () => {
    if (busy) return;

    busy = true;
    hideError(error);

    try {
      await change(!ignored);
      ignored = !ignored;
      render();
    } catch (e) {
      showError(error, e);
    } finally {
      busy = false;
    }
  });

  render();
  link.hidden = false;
}
