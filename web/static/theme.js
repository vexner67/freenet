// The theme of "Стиль", put on before the page is first painted. This is
// the one script of the site that is not a module: a module runs after
// the page is parsed, and a dark page would flash light until then. It
// runs where the parser meets it, right after style.css, and writes the
// stylesheet of the theme into the document there, so the browser waits
// for it as it waits for style.css. layout.js keeps the same link once
// the settings are known; the keys and the themes are the ones it names.
// The stylesheets are named whole: the server puts their versions into
// the paths, the same ones layout.js gets.
(() => {
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
