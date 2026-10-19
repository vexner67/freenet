// The theme of "Стиль", put on before the page is first painted. This is
// the one script of the site that is not a module: a module runs after
// the page is parsed, and a dark page would flash light until then. It
// runs where the parser meets it, right after style.css, and writes the
// stylesheet of the theme into the document there, so the browser waits
// for it as it waits for style.css. layout.js keeps the same link once
// the settings are known; the keys and the themes are the ones it names.
// The stylesheets are named whole: the server puts their versions into
// the paths, the same ones layout.js gets.
//
// It also hides the page below the header until the modules have drawn it
// (layout.js shows it): a page comes up whole, not block by block. A slow
// server or a broken script shows what there is after a while.
(() => {
  const root = document.documentElement;
  root.classList.add('pending');
  setTimeout(() => root.classList.remove('pending'), 1200);
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
  };

  let theme = null;
  try {
    theme = localStorage.getItem('freenet.theme')
      ?? JSON.parse(localStorage.getItem('freenet.interface'))?.theme;
  } catch {
    // Without storage the page wears the theme layout.js gives it.
  }

  if (typeof theme !== 'string' || !Object.hasOwn(themes, theme)) return;

  document.write(`<link id="theme" rel="stylesheet" href="${themes[theme]}">`);
})();
