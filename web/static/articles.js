// Articles of the editor, as "Статьи" of VK: long reads of blocks —
// paragraphs, headings, quotes, lists, code, photos with captions and
// dividers — whose text is spans written bold, italic, struck out or as
// links. This module draws the blocks (for reading and, the same tags, for
// the editor), reads them back out of the editor and draws the card of an
// article in a post that links to it.

import { api } from './api.js';
import { el, externalLink } from './layout.js';
import { plural } from './labels.js';
import { tr } from './i18n.js';

// articlePath is the page of an article; articleURL its whole address,
// which a post links to.
export const articlePath = (id) => `/article/${encodeURIComponent(id)}`;
export const articleURL = (id) => location.origin + articlePath(id);

const uuid = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';

// articleIn finds the link to an article of this site in the text of a
// post: { id, url }, null for none.
export function articleIn(text) {
  if (!text) return null;

  const host = location.host.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const found = new RegExp(`https?://${host}/article/(${uuid})`, 'i').exec(text);

  return found ? { id: found[1].toLowerCase(), url: found[0] } : null;
}

// minutesText is how long an article is read: "5 мин. чтения".
export const minutesText = (minutes) => tr('{0} мин. чтения', minutes);

// spanNodes are the spans of a block as inline nodes: a link outside, then
// the strike, the italic and the bold; a line break is a <br>. In the
// editor (editing) a link is a plain <a> that does not open.
function spanNodes(spans = [], editing = false) {
  return spans.map((s) => {
    const lines = s.text.split('\n');
    let node = el('span', {}, ...lines.flatMap((line, i) => (i ? [el('br'), line] : [line])));
    if (s.bold) node = el('b', {}, node);
    if (s.italic) node = el('i', {}, node);
    if (s.strike) node = el('s', {}, node);
    if (s.link) node = editing ? el('a', { href: s.link }, node) : externalLink(s.link, [node]);
    return node;
  });
}

// photoFigure is a photo of an article with its caption; in the editor the
// caption is a field and a cross takes the photo away.
export function photoFigure(photo, caption = '', editing = false) {
  const img = el('img', { src: photo.large, alt: caption, loading: 'lazy' });
  if (!editing) {
    return el('figure', { className: 'article-photo' }, img, caption ? el('figcaption', { textContent: caption }) : null);
  }

  const figure = el('figure', { className: 'article-photo', contentEditable: 'false' }, img,
    el('input', { type: 'text', className: 'article-caption', maxLength: 256, value: caption, placeholder: tr('Подпись к фотографии') }),
    el('button', { type: 'button', className: 'article-photo-remove', title: tr('Убрать фотографию'), textContent: '×', onclick: () => figure.remove() }));
  figure.dataset.photo = photo.id;
  figure.dataset.large = photo.large;
  return figure;
}

// blockNodes draw the blocks of an article; the list items next to each
// other are one list.
export function blockNodes(blocks, editing = false) {
  const nodes = [];
  let list = null;
  for (const b of blocks) {
    const tag = { bullet: 'ul', number: 'ol' }[b.kind];
    if (!tag) list = null;

    switch (b.kind) {
      case 'heading': nodes.push(el('h2', {}, ...spanNodes(b.spans, editing))); break;
      case 'subheading': nodes.push(el('h3', {}, ...spanNodes(b.spans, editing))); break;
      case 'quote': nodes.push(el('blockquote', {}, ...spanNodes(b.spans, editing))); break;
      case 'code': nodes.push(el('pre', { textContent: (b.spans ?? []).map((s) => s.text).join('') })); break;
      case 'divider': nodes.push(el('hr')); break;
      case 'photo': if (b.photo) nodes.push(photoFigure(b.photo, b.caption, editing)); break;
      case 'bullet':
      case 'number':
        if (list?.tagName.toLowerCase() !== tag) {
          list = el(tag);
          nodes.push(list);
        }
        list.append(el('li', {}, ...spanNodes(b.spans, editing)));
        break;
      default: nodes.push(el('p', {}, ...spanNodes(b.spans, editing)));
    }
  }

  return nodes;
}

const webLink = /^https?:\/\/[^\s/?#]+/i;

// inlineSpans read the spans of a node of the editor: <b>, <strong> and
// bold styles are bold, and so on; a <br> is a line break.
function inlineSpans(node, marks = {}, out = []) {
  for (const child of node.childNodes) {
    if (child.nodeType === Node.TEXT_NODE) {
      if (child.data) out.push({ text: child.data.replace(/ /g, ' '), ...marks });
      continue;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) continue;

    const tag = child.tagName.toLowerCase();
    if (tag === 'br') {
      out.push({ text: '\n', ...marks });
      continue;
    }

    const style = child.style;
    const next = { ...marks };
    if (tag === 'b' || tag === 'strong' || style?.fontWeight === 'bold' || Number(style?.fontWeight) >= 600) next.bold = true;
    if (tag === 'i' || tag === 'em' || style?.fontStyle === 'italic') next.italic = true;
    if (tag === 's' || tag === 'strike' || tag === 'del' || style?.textDecorationLine?.includes('line-through')) next.strike = true;
    if (tag === 'a' && webLink.test(child.getAttribute('href') ?? '')) next.link = child.getAttribute('href');
    inlineSpans(child, next, out);
  }

  return out;
}

// cleanSpans join the spans written the same way and drop the line breaks
// at the end, which an editor leaves in an empty line.
function cleanSpans(spans) {
  const out = [];
  for (const s of spans) {
    const span = { text: s.text };
    for (const mark of ['bold', 'italic', 'strike', 'link']) if (s[mark]) span[mark] = s[mark];
    const last = out.at(-1);
    if (last && ['bold', 'italic', 'strike', 'link'].every((m) => last[m] === span[m])) last.text += span.text;
    else out.push(span);
  }

  while (out.length) {
    const last = out.at(-1);
    last.text = last.text.replace(/\n+$/, '');
    if (last.text) break;
    out.pop();
  }

  return out;
}

const textBlock = (kind, node) => ({ kind, spans: cleanSpans(inlineSpans(node)) });

// readBlocks are the blocks the editor holds, as blockNodes draws them; the
// API takes a photo by its ID.
export function readBlocks(root) {
  const blocks = [];
  const walk = (node, quote = false) => {
    if (node.nodeType === Node.TEXT_NODE) {
      if (node.data.trim()) blocks.push({ kind: quote ? 'quote' : 'paragraph', spans: cleanSpans([{ text: node.data }]) });
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;

    const tag = node.tagName.toLowerCase();
    if (tag === 'figure' && node.dataset.photo) {
      blocks.push({ kind: 'photo', photo: { id: node.dataset.photo, large: node.dataset.large }, caption: node.querySelector('.article-caption')?.value.trim() ?? '' });
    } else if (tag === 'hr') {
      blocks.push({ kind: 'divider' });
    } else if (tag === 'pre') {
      blocks.push({ kind: 'code', spans: node.textContent.trim() ? [{ text: node.textContent.replace(/\n+$/, '') }] : [] });
    } else if (tag === 'h1' || tag === 'h2') {
      blocks.push(textBlock('heading', node));
    } else if (/^h[3-6]$/.test(tag)) {
      blocks.push(textBlock('subheading', node));
    } else if (tag === 'ul' || tag === 'ol') {
      for (const li of node.children) blocks.push(textBlock(tag === 'ul' ? 'bullet' : 'number', li));
    } else if (tag === 'blockquote') {
      // A quote of several paragraphs is a quote each.
      if ([...node.children].some((c) => /^(p|div)$/i.test(c.tagName))) [...node.childNodes].forEach((c) => walk(c, true));
      else blocks.push(textBlock('quote', node));
    } else if ((tag === 'div' || tag === 'p') && [...node.children].some((c) => /^(p|div|h\d|ul|ol|pre|blockquote|figure|hr)$/i.test(c.tagName))) {
      // What a browser nests, read as it shows it.
      [...node.childNodes].forEach((c) => walk(c, quote));
    } else {
      blocks.push(textBlock(quote ? 'quote' : 'paragraph', node));
    }
  };

  [...root.childNodes].forEach((c) => walk(c));

  return blocks.filter((b) => !b.spans || b.spans.some((s) => s.text.trim()) || b.kind === 'code' && b.spans.length);
}

// The briefs of the articles the posts of a page link to, asked once each.
const briefs = new Map();

function brief(id) {
  if (!briefs.has(id)) briefs.set(id, api.article(id, true).catch(() => null));
  return briefs.get(id);
}

// cardParts are what the card of an article shows: its cover darkened
// under the title, the beginning of the text and how long it is read.
function cardParts(card, article) {
  card.classList.remove('loading');
  card.classList.toggle('with-cover', Boolean(article.cover));
  if (article.cover) card.style.setProperty('--article-cover', `url("${CSS.escape(article.cover.large)}")`);
  card.replaceChildren(
    el('span', { className: 'article-card-shade' }),
    el('span', { className: 'article-card-text' },
      el('span', { className: 'article-card-label', textContent: tr('Статья') }),
      el('b', { className: 'article-card-title', textContent: article.title }),
      article.excerpt ? el('span', { className: 'article-card-excerpt', textContent: article.excerpt }) : null,
      el('span', { className: 'article-card-meta', textContent: minutesText(article.minutes) }),
      el('span', { className: 'article-card-read', textContent: tr('Читать') })),
  );

  return card;
}

// articleCard is the card of an article at hand, as a list shows it.
export function articleCard(article) {
  return cardParts(el('a', { className: 'article-card', href: articlePath(article.id) }), article);
}

// articleCardNode is the card of the article a post links to: it comes
// when the article is read.
export function articleCardNode(found) {
  const card = el('a', { className: 'article-card loading', href: articlePath(found.id) },
    el('span', { className: 'article-card-label', textContent: tr('Статья') }));

  // A deleted article leaves its post saying so, not empty.
  brief(found.id).then((article) => {
    if (article) cardParts(card, article);
    else card.replaceWith(el('div', { className: 'post-shared deleted', textContent: tr('Статья удалена.') }));
  });

  return card;
}

// articlesCount is "3 статьи".
export const articlesCount = (n) => `${n} ${plural(n, ['статья', 'статьи', 'статей'])}`;
