package rest

import (
	"net/http"
	"strconv"
)

// sandboxPath is the page in which posts and messages run the HTML
// documents they attach: a game, say, written by a person.
const sandboxPath = "/sandbox"

// sandboxCSP makes the page a sandbox of its own: scripts run, but in an
// opaque origin, so they reach neither the tokens of the site nor its
// cookies, nor do they take the page over, open windows or send forms.
// They load nothing from the network either, nor send anything there:
// what a page needs, it carries inline or as data: and blob: addresses.
// Only a page of the site may frame it.
const sandboxCSP = "sandbox allow-scripts allow-pointer-lock; default-src 'none'; " +
	"script-src 'unsafe-inline' 'unsafe-eval'; style-src 'unsafe-inline'; " +
	"img-src data: blob:; media-src data: blob:; font-src data:; worker-src blob:; " +
	"frame-ancestors 'self'; base-uri 'none'; form-action 'none'"

// sandboxPage waits for the page it is to show from the page that framed
// it, which it tells it is ready, and becomes that page. Nothing but that
// one string from its parent is taken, and after it only the parent's
// replies. The keys a game plays with, the arrows and the space, then move
// the game only: unless a field takes them, they no longer scroll the page
// around the frame.
//
// window.freenet is what the page may ask the site, which answers as the
// one who plays (htmlapp.js): save(data) keeps any JSON of up to 64 KB,
// load() gives it back (null without one), score(points) keeps a result
// and gives the best. Each returns a promise. The globals of the window
// outlive document.open, its listeners do not: the replies are listened
// to after the page is written.
const sandboxPage = `<!DOCTYPE html>
<meta charset="utf-8">
<script>
addEventListener('message', function (e) {
  if (e.source !== parent || typeof e.data !== 'string') return;
  var waiting = {}, next = 0;
  function ask(kind, value) {
    return new Promise(function (resolve, reject) {
      next++;
      waiting[next] = [resolve, reject];
      parent.postMessage({ freenet: kind, id: next, value: value }, '*');
    });
  }
  window.freenet = Object.freeze({
    save: function (data) {
      var json;
      try { json = JSON.stringify(data); } catch (err) { return Promise.reject(err); }
      if (json === undefined) return Promise.reject(new TypeError('nothing to save'));
      return ask('save', json).then(function () {});
    },
    load: function () {
      return ask('load').then(function (json) { return json === null ? null : JSON.parse(json); });
    },
    score: function (points) { return ask('score', Number(points)); }
  });
  document.open();
  document.write(e.data);
  document.close();
  addEventListener('message', function (r) {
    var d = r.data;
    if (r.source !== parent || !d || d.freenet !== 'reply' || !waiting[d.id]) return;
    var w = waiting[d.id];
    delete waiting[d.id];
    if (d.error) w[1](new Error(d.error)); else w[0](d.value);
  });
  addEventListener('keydown', function (k) {
    var t = k.target;
    if (t && t.closest && t.closest('input, textarea, select, [contenteditable]')) return;
    if ([' ', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'PageUp', 'PageDown'].indexOf(k.key) >= 0) k.preventDefault();
  });
}, { once: true });
parent.postMessage('freenet-sandbox-ready', '*');
</script>
`

// sandbox serves the sandbox page; it is the same for everyone, so it is
// cached as the pages are.
func sandbox(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Security-Policy", sandboxCSP)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(sandboxPage)))
	w.Header().Set("Cache-Control", cacheRevalidate)
	_, _ = w.Write([]byte(sandboxPage)) // the client may have gone; nothing to do
}
