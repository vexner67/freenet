// Answers and reactions of messages in a dialog and a chat, as on VK:
// "Ответить" puts a line over the form with whom one answers, the answer
// shows over its words the message it answers, a click leading to it; and
// anyone in the conversation puts one reaction of six on a message, shown
// under it with how many put each. The reactions are smileys of the site,
// the same as those of the words.

import { REACTIONS, formatDateTime, idTime, primeNameClass, quoteText, reactionCounts, reacted } from './labels.js';
import { $, avatarURL, el, fullName, showError, smileyImage, smileyNodes } from './layout.js';
import { tr } from './i18n.js';

// quoteNode is the block over an answer's words, as openvk's
// reply-msg-container (components/message.js): the small photo, the name
// and the date of the message it answers, its words or what it shows, and
// "в ответ на сообщение" whose; text is its words as the page knows them,
// open leads to that message. An author with Prime has the name in its
// colour.
export function quoteNode(q, text, open) {
  const said = quoteText(q, text);
  const when = idTime(q.id);

  return el('a', {
    className: 'reply-msg-container',
    title: tr('Перейти к сообщению'),
    onclick: () => open(q.id),
  },
  q.author && el('div', { className: 'reply-msg-head' },
    el('img', { className: 'reply-avatar', src: avatarURL(q.author, 'small'), alt: '' }),
    el('b', { className: ['reply-author-name', primeNameClass(q.author)].filter(Boolean).join(' '), textContent: fullName(q.author) }),
    when && el('span', { className: 'reply-date', textContent: formatDateTime(when) })),
  el('div', { className: q.author ? 'reply-text' : 'reply-text reply-no-text' }, ...smileyNodes(said)),
  q.author && el('div', { className: 'reply-msg-header', textContent: tr('в ответ на сообщение {0}', fullName(q.author)) }));
}

// replyBar runs the line over the form: start(m, author, text) answers m,
// cancel stops; replyingTo is the message answered, null for none.
export function replyBar() {
  let replying = null;

  const cancel = () => {
    replying = null;
    $('replying').hidden = true;
  };
  $('replying-cancel').addEventListener('click', cancel);

  return {
    start(m, author, text) {
      replying = m;
      $('replying-name').textContent = tr('в ответ на сообщение {0}', fullName(author)) + ':';
      $('replying-text').replaceChildren(...smileyNodes(quoteText({ ...m, author, photoCount: m.photos?.length ?? 0, albumPhoto: Boolean(m.albumPhoto), post: Boolean(m.post) }, text)));
      $('replying').hidden = false;
      $('text').focus();
    },
    cancel,
    replyingTo: () => replying,
  };
}

// reactionNodes are the reactions under a message and, while picking,
// the six to choose from; react(m, reaction) puts the caller's.
export function reactionNodes(m, userID, picking, react) {
  const counts = reactionCounts(m.reactions, userID);
  const mine = (m.reactions ?? []).find((r) => r.userId === userID)?.reaction ?? '';
  const chip = (r, count) => el('button', {
    type: 'button',
    className: r.name === mine ? 'im-reaction mine' : 'im-reaction',
    title: r.title,
    onclick: () => react(m, r.name === mine ? '' : r.name),
  }, smileyImage(r), count ? el('span', { textContent: count }) : null);

  return [
    counts.length > 0 && el('div', { className: 'im-reactions' }, ...counts.map((r) => chip(r, String(r.count)))),
    picking && el('div', { className: 'im-reaction-picker' }, ...REACTIONS.map((r) => chip(r, ''))),
  ].filter(Boolean);
}

// reactWith makes react for a page: the node of the message shows the
// reaction at once and goes back when the server says no; put asks the
// server, error is where to say why.
export function reactWith({ shown, userID, put, error }) {
  return async (m, reaction) => {
    const node = shown.get(m.id);
    if (!node) return;

    const before = node.message.reactions;
    node.picking = false;
    node.message = { ...node.message, reactions: reacted(before, userID, reaction) };
    node.render();

    try {
      await put(m.id, reaction);
    } catch (e) {
      node.message = { ...node.message, reactions: before };
      node.render();
      showError(error, e);
    }
  };
}
