// The theme of "Стиль", put on before the page is first painted. This is
// the one script of the site that is not a module: a module runs after
// the page is parsed, and a dark page would flash light until then. It
// runs where the parser meets it, right after the stylesheet of the page
// (style.css, or m.css on a phone), and writes the stylesheets of the
// theme and of "Оформление" into the document there, so the browser waits
// for them as it waits for the page's. layout.js keeps the same links
// once the settings are known; the keys and the files are the ones it
// names.
// The stylesheets are named whole: the server puts their versions into
// the paths, the same ones layout.js gets.
//
// It also hides the page below the header until the modules have drawn it
// (layout.js shows it): a page comes up whole, not block by block. A slow
// server or a broken script shows what there is after a while.
(() => {
  const root = document.documentElement;
  root.classList.add('pending');
  setTimeout(() => root.classList.add('slow'), 400);
  setTimeout(() => root.classList.remove('pending'), 1200);
})();

// The New Year decoration (newyear.js): the garland and the hat on the
// logo are style.css's, put on here before the page is painted when the
// browser remembers the site wears them.
(() => {
  const root = document.documentElement;
  if (root.classList.contains('aui')) return;
  try {
    if (JSON.parse(localStorage.getItem('freenet.decor'))?.festive === true) root.classList.add('festive');
  } catch {
    // newyear.js asks the server.
  }
})();

(() => {
  const root = document.documentElement;
  // "Админ-панель" (admin.html) wears none of the themes: it has AUI's
  // dark mode of its own, as openvk's, and its sidebar may be folded.
  if (root.classList.contains('aui')) {
    let dark = null;
    let folded = false;
    try {
      dark = localStorage.getItem('freenet.admintheme');
      folded = localStorage.getItem('freenet.adminsidebar') === 'folded';
    } catch {
      // Without storage the panel follows the system.
    }
    if (dark === null) dark = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    root.classList.toggle('aui-theme-dark', dark === 'dark');
    root.classList.toggle('aui-collapsed', folded);
    return;
  }

  const themes = {
    blue: '/static/themes/blue.css',
    dark: '/static/themes/dark.css',
    gold: '/static/themes/gold.css',
  };
  // "Оформление": the site of 2010 needs nothing; the modern one is a
  // stylesheet over the page and its theme, and a class of the page.
  const designs = {
    modern: '/static/modern.css',
  };

  let theme = null;
  let design = null;
  try {
    theme = localStorage.getItem('freenet.theme');
    design = localStorage.getItem('freenet.design');
    const ui = JSON.parse(localStorage.getItem('freenet.interface'));
    theme ??= ui?.theme;
    design ??= ui?.design;
  } catch {
    // Without storage the page wears the look layout.js gives it.
  }

  const known = (map, key) => typeof key === 'string' && Object.hasOwn(map, key);
  let markup = '';
  if (known(themes, theme)) markup += `<link id="theme" rel="stylesheet" href="${themes[theme]}">`;
  if (known(designs, design)) {
    root.classList.add(design);
    if (theme === 'dark') root.classList.add('theme-dark');
    markup += `<link id="design" rel="stylesheet" href="${designs[design]}">`;
  }
  if (!markup) return;

  // The pages let only named policies make markup (Trusted Types in the
  // CSP of the gateway): this one writes nothing but the links above.
  const policy = globalThis.trustedTypes?.createPolicy('freenet-theme', {
    createHTML: (html) => {
      if (html !== markup) throw new TypeError('not a theme link');
      return html;
    },
  });
  document.write(policy ? policy.createHTML(markup) : markup);
})();
